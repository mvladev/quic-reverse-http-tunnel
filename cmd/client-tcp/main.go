package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"time"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/mvladev/quic-reverse-http-tunnel/internal/pipe"
)

// We start a server echoing data on the first stream the client opens,
// then connect with a client, send the message, and wait for its receipt.
func main() {
	err := clientMain()
	if err != nil {
		panic(err)
	}
}

const (
	ListenerCloseCode quic.ErrorCode = 100
)

func clientMain() error {
	var ca, cert, key, server, upstream string

	flag.StringVar(&ca, "ca-file", "", "ca file")
	flag.StringVar(&cert, "cert-file", "", "client cert file")
	flag.StringVar(&key, "cert-key", "", "client key file")
	flag.StringVar(&server, "server", "127.0.0.1:9999", "host:port of the quic server")
	flag.StringVar(&upstream, "upstream", "",
		"host:port of the proxy server which will send traffic to the correct upstream. e.g. www.example.com:443")

	flag.Parse()

	if upstream == "" {
		return fmt.Errorf("must specify upstream host")
	}

	data, err := ioutil.ReadFile(ca)
	if err != nil {
		return fmt.Errorf("could not read certificate authority: %s", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(data) {
		return fmt.Errorf("could not append certificate data")
	}

	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return fmt.Errorf("could not read client certificates: %s", err)
	}

	tlsConf := &tls.Config{
		ServerName:   "quic-tunnel-server",
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      certPool,
		NextProtos:   []string{"quic-echo-example"},
	}

	conf := &quic.Config{
		KeepAlive:                             true,
		HandshakeTimeout:                      time.Second * 2,
		MaxIdleTimeout:                        time.Second * 5,
		MaxReceiveStreamFlowControlWindow:     246 * (1 << 20), // 276 MB
		MaxIncomingStreams:                    10000,
		MaxReceiveConnectionFlowControlWindow: 500 * (1 << 20), // 512 MB,
		MaxIncomingUniStreams:                 10000,
	}

	ctx := context.Background()

	for {
		log.Println("dialing quic server...")

		session, err := quic.DialAddrContext(ctx, server, tlsConf, conf)
		if err != nil {
			// TODO this needs backoff
			log.Printf("could not dial server %+v", err)

			continue
		}

		log.Println("connected to quic server")

		for {
			src, err := session.AcceptStream(ctx)
			if err != nil {
				log.Printf("Accept error: %s", err)
				session.CloseWithError(ListenerCloseCode, "die")

				// Hack
				time.Sleep(time.Second * 5)

				continue
			}

			log.Println("got a new stream from the server")

			dst, err := net.Dial("tcp", upstream)
			if err != nil {
				log.Printf("can't dial %q %+v", upstream, err)
				if src != nil {
					src.Close()
				}
				if dst != nil {
					dst.Close()
				}

				continue
			}

			go pipe.Request(src, dst)
		}
	}
}
