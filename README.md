# What it does

It's a reverse HTTP Tunnel using QUIC:

```text
K8S apiserver / curl --- TCP ----> [proxy-server] ---- QUIC ----> [proxy-agent]---TCP--> [kubelet]
```

1. the proxy-server listens for `tcp` (no HTTP server running) and `quic`.
1. The proxy-agent talks to the server and opens a `quic` session.
1. It starts a HTTP tunnel server that listens on that session for new streams.
1. When the API server / curl talks to the proxy-server, it creates a new `quic` stream and sends the data to the proxy-agent.
1. The HTTP server in the proxy-agent that listens on new quic streams accepts the stream, opens TCP connection to the requested host (from the CONNECT) and pipes the data back.

## Building and running

Run the server:

```console
$ go run cmd/server/main.go --listen-tcp 0.0.0.0:10443 --listen-quic 0.0.0.0:8888 --cert-file certs/tls.crt --cert-key certs/tls.key
2020/11/01 02:11:39 quick listener on 0.0.0.0:8888
2020/11/01 02:11:39 tcp listener on 0.0.0.0:10443
2020/11/01 02:11:39 waiting for new quic client session
2020/11/01 02:11:39 waiting for tcp client connections
```

in another terminal run the client:

```console
$ go run cmd/client/main.go --server=localhost:8888 --ca-file certs/ca.crt
2020/11/01 02:13:31 dialing quic server...
2020/11/01 02:13:31 starting http server
```

and in third try to access it:

```console
curl -p --proxy localhost:10443 http://www.example.com
```

Docker file:

```console
docker build --target=server -t my-tag/quic-server .
docker build --target=client -t my-tag/quic-client .
```
