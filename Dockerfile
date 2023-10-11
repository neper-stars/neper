FROM quay.orus.io/docker_mirror/golang:1.21.0-bullseye as builder

COPY . .

RUN unset GOPATH && CGO_ENABLED=0 GOARCH=amd64 go build -v -a -tags netgo -ldflags "-w -extldflags "-static"" -buildvcs=false -o /usr/local/bin/neper ./cmd/neper

FROM quay.orus.io/cloudcrane/alpine:3.18

COPY --from=builder /usr/local/bin/neper /usr/local/bin/neper
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN apk add --no-cache wine xvfb\
	&& adduser \
	    --disabled-password \
	    --gecos "" \
	    --uid 1337 \
	    neper

USER neper
WORKDIR /home/neper

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
