package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/fen4o/quic/internal/pipe"
	quic "github.com/lucas-clemente/quic-go"
)

func main() {
	log.Fatal(startServeer())
}

type clients struct {
	mu       sync.RWMutex
	sessions []quic.Session
	next     int
	random   *rand.Rand
}

// nextSession returns a random session at round-robin
func (c *clients) nextSession() (quic.Session, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.sessions) == 0 {
		return nil, fmt.Errorf("no client connections available")
	}

	sc := c.sessions[c.next]
	c.next = (c.next + 1) % len(c.sessions)

	return sc, nil
}

// Start a server that echos all data on the first stream opened by the client
func startServeer() error {
	var cert, key, quickListener, tcpListener string

	flag.StringVar(&cert, "cert-file", "", "cert file")
	flag.StringVar(&key, "cert-key", "", "key file")
	flag.StringVar(&quickListener, "listen-quic", "0.0.0.0:8888", "listen for quic")
	flag.StringVar(&tcpListener, "listen-tcp", "0.0.0.0:8443", "listen for tcp")

	flag.Parse()

	c := clients{
		sessions: []quic.Session{},
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	conf := &quic.Config{
		HandshakeTimeout:                      time.Second * 2,
		MaxIdleTimeout:                        time.Second * 5,
		MaxReceiveStreamFlowControlWindow:     246 * (1 << 20), // 276 MB
		MaxIncomingStreams:                    10000,
		MaxReceiveConnectionFlowControlWindow: 500 * (1 << 20), // 512 MB,
		MaxIncomingUniStreams:                 10000,
	}

	log.Printf("quick listener on %s", quickListener)

	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		panic(err)
	}

	tlsc := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}

	ql, err := quic.ListenAddr(quickListener, tlsc, conf)
	if err != nil {
		return err
	}

	log.Printf("tcp listener on %s", tcpListener)

	l, err := net.Listen("tcp4", tcpListener)
	if err != nil {
		log.Fatalf("can't listen for tcp on %s: %v", tcpListener, err)
	}

	ctx := context.Background()

	go func() {
		for {
			log.Println("waiting for tcp client connections")

			src, err := l.Accept()
			if err != nil {
				log.Fatalf("Accept error: %s", err)
			}

			fmt.Printf("accepted TCP client connection %s\n", src.RemoteAddr())

			s, err := c.nextSession()
			if err != nil {
				log.Printf("could not process tcp connection %+v", err)
				src.Close()

				continue
			}

			stream, err := s.OpenStreamSync(ctx)
			if err != nil {
				log.Printf("cannot open stream %+v", err)

				continue
			}

			fmt.Printf("opened QUICK Stream connection %d\n", stream.StreamID())

			go pipe.Request(src, stream)
		}
	}()

	for {
		log.Println("waiting for new quic client session")

		sess, err := ql.Accept(ctx)
		if err != nil {
			log.Printf("got error when accepting the connection %+v", err)

			continue
		}

		log.Println("got a quic client session")

		go func(s quic.Session) {
			c.mu.Lock()
			c.sessions = append(c.sessions, s)
			c.mu.Unlock()

			<-sess.Context().Done()
			fmt.Println("session closed. Opsie.")

			c.mu.RLock()
			for i := 0; i < len(c.sessions); i++ {
				if c.sessions[i] != s {
					continue
				}

				c.sessions[i] = c.sessions[len(c.sessions)-1]
				c.sessions = c.sessions[:len(c.sessions)-1]

				if slen := len(c.sessions); slen == 0 {
					c.next = 0
				} else {
					c.next = c.random.Intn(slen)
				}

				break
			}
			c.mu.RUnlock()
		}(sess)
	}
}
