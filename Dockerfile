#############      builder       #############
FROM golang:1.15.3 AS builder

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install ./...

#############      server     #############
FROM alpine:3.12.1 AS server

RUN apk add --update tzdata

COPY --from=builder /go/bin/server /server

WORKDIR /

ENTRYPOINT ["/server"]

############# client #############
FROM alpine:3.12.1 AS client

RUN apk add --update tzdata

COPY --from=builder /go/bin/client /client

WORKDIR /

ENTRYPOINT ["/client"]

############# client-tcp #############
FROM alpine:3.12.1 AS client-tcp

RUN apk add --update tzdata

COPY --from=builder /go/bin/client-tcp /client-tcp

WORKDIR /

ENTRYPOINT ["/client-tcp"]
