package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	quic "github.com/lucas-clemente/quic-go"
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

var (
	_ net.Listener = &listener{}
	_ net.Conn     = &conn{}
)

// listener implements net.Listener
type listener struct {
	session quic.Session
	ctx     context.Context
}

func (h *listener) Accept() (net.Conn, error) {
	s, err := h.session.AcceptStream(h.ctx)
	if err != nil {
		return nil, err
	}

	return &conn{
		Stream: s,
		local:  h.session.LocalAddr(),
		remote: h.session.RemoteAddr(),
	}, nil
}

func (h *listener) Close() error {
	return h.session.CloseWithError(ListenerCloseCode, "die")
}

func (h *listener) Addr() net.Addr {
	return nil
}

// conn implements net.Conn.
type conn struct {
	quic.Stream
	local, remote net.Addr
}

func (h *conn) LocalAddr() net.Addr {
	return h.local
}

func (h *conn) RemoteAddr() net.Addr {
	return h.remote
}

type connectServer struct {
	connectResponse []byte
}

func (c *connectServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request for host %s, method %s,  useragent %s\n", r.Host, r.Method, r.UserAgent())

	if r.Method != http.MethodConnect {
		http.Error(w, "this proxy only supports CONNECT passthrough", http.StatusMethodNotAllowed)

		return
	}

	// Connect to Remote.
	dst, err := net.Dial("tcp", r.RequestURI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	defer dst.Close()

	// Upon success, we respond a 200 status code to client.
	if _, err := w.Write(c.connectResponse); err != nil {
		log.Printf("could not write 200 response %+v", err)

		return
	}

	// Now, Hijack the writer to get the underlying net.Conn.
	// Which can be either *tcp.Conn, for HTTP, or *tls.Conn, for HTTPS.
	src, bio, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
	defer src.Close()

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		// Copy unprocessed buffered data from the client to dst so we can use src directly.
		if n := bio.Reader.Buffered(); n > 0 {
			n64, err := io.CopyN(dst, bio, int64(n))
			if n64 != int64(n) || err != nil {
				log.Println("io.CopyN:", n64, err)

				return
			}
		}

		// src -> dst
		if _, err := io.Copy(dst, src); err != nil {
			log.Printf("cant copy from source to destination %+v", err)
		}
	}()

	go func() {
		defer wg.Done()

		// dst -> src
		if _, err := io.Copy(src, dst); err != nil {
			log.Printf("cant copy from destination to source %+v", err)
		}
	}()

	wg.Wait()
}

func clientMain() error {
	var ca, server string

	flag.StringVar(&ca, "ca-file", "", "ca file")
	flag.StringVar(&server, "server", "127.0.0.1:9999", "host:port of the quic server")

	flag.Parse()

	data, err := ioutil.ReadFile(ca)
	if err != nil {
		return fmt.Errorf("could not read certificate authority: %s", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(data) {
		return fmt.Errorf("could not append certificate data")
	}

	tlsConf := &tls.Config{
		ServerName: "localhost",
		RootCAs:    certPool,
		NextProtos: []string{"quic-echo-example"},
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

		go func() {
			<-session.Context().Done()
			log.Println("session closed. Opsie.")
		}()

		log.Println("starting http server")

		err = http.Serve(&listener{session: session, ctx: ctx}, &connectServer{
			connectResponse: []byte("HTTP/1.1 200 OK\r\n\r\n"),
		})
		if err != nil {
			log.Printf("could not close listen on http server %+v", err)
		}
	}
}
