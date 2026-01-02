FROM golang:1.25-bookworm AS builder

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 GOARCH=amd64 go build -v -a -tags netgo -ldflags "-w -extldflags \"-static\"" -buildvcs=false -o /usr/local/bin/neper ./cmd/neper

FROM debian:bookworm-slim

# Install wine with 32-bit support, xvfb, and gosu for privilege dropping
# Enable i386 architecture for 32-bit wine support
# Stars! is a 16-bit Windows 3.1 application, which requires:
# - A 32-bit wine prefix (WINEARCH=win32)
# - The wine32 package for 32-bit/16-bit Windows support
# - Linux kernel with CONFIG_X86_16BIT enabled (host kernel requirement)
RUN dpkg --add-architecture i386 \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends \
		wine \
		wine32 \
		xvfb \
		gosu \
		ca-certificates \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd -m -u 1337 -s /bin/bash neper

# Set environment for 32-bit wine prefix (required for 16-bit apps like Stars!)
ENV WINEARCH=win32

COPY --from=builder /usr/local/bin/neper /usr/local/bin/neper
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

VOLUME /home/neper
WORKDIR /home/neper

EXPOSE 33670

# Runs as root initially, entrypoint drops to neper user after fixing permissions
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["serve", "--auto-migrate"]
