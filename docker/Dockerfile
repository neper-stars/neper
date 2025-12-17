FROM registry.orus.io/orus-io/docker_mirror/golang:1.25.4-trixie as builder

COPY . .

RUN unset GOPATH && CGO_ENABLED=0 GOARCH=amd64 go build -v -a -tags netgo -ldflags "-w -extldflags "-static"" -buildvcs=false -o /usr/local/bin/neper ./cmd/neper

FROM registry.orus.io/orus-io/docker_mirror/alpine:3.23.0

COPY --from=builder /usr/local/bin/neper /usr/local/bin/neper
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh

RUN apk add --no-cache wine xvfb su-exec \
	&& chmod +x /usr/local/bin/entrypoint.sh \
	&& adduser \
	    --disabled-password \
	    --gecos "" \
	    --uid 1337 \
	    neper \

VOLUME /home/neper
WORKDIR /home/neper

EXPOSE 33670

# Runs as root initially, entrypoint drops to neper user after fixing permissions
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["serve", "--auto-migrate"]
