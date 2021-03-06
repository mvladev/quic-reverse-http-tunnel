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

The proxy can also run as a simple passthrough proxy via `client-tcp`
## Building and running

Run the server:

```console
$ go run cmd/server/main.go --listen-tcp 0.0.0.0:10443 --listen-quic 0.0.0.0:8888 --cert-file certs/tls.crt --cert-key certs/tls.key --client-ca-file certs/ca.crt --v=2
2020/11/01 02:11:39 quick listener on 0.0.0.0:8888
2020/11/01 02:11:39 tcp listener on 0.0.0.0:10443
2020/11/01 02:11:39 waiting for new quic client session
2020/11/01 02:11:39 waiting for tcp client connections
```

in another terminal run the client:

```console
$ go run cmd/client/main.go --server=localhost:8888 --ca-file certs/ca.crt --cert-file certs/client.crt --cert-key certs/client.key --v=2
2020/11/01 02:13:31 dialing quic server...
2020/11/01 02:13:31 starting http server
```

and in third try to access it:

```console
curl -p --proxy localhost:10443 http://www.example.com
```

If you want to test the passthrough proxy instead:

```console
$ go run cmd/client-tcp/main.go --server=localhost:8888 --ca-file certs/ca.crt --cert-file certs/client.crt --cert-key certs/client.key --v=2 --upstream=www.example.com:80
2020/11/25 12:07:07 dialing quic server...
2020/11/25 12:07:07 connected to quic server
```

## Docker images

Docker images are available at:

- `ghcr.io/mvladev/quic-reverse-http-tunnel/quic-server:v0.1.2`
- `ghcr.io/mvladev/quic-reverse-http-tunnel/quic-client:v0.1.2`
- `ghcr.io/mvladev/quic-reverse-http-tunnel/quic-client-tcp:v0.1.2`

or or at the `latest` tag.
