#!/bin/sh

# Allow configurable UID/GID via environment variables
# Defaults to 1000:1000 if not set
NEPER_UID=${NEPER_UID:-1000}
NEPER_GID=${NEPER_GID:-1000}

# Generate config if it doesn't exist (runs as root if needed)
if [ ! -f /etc/neper/neper.conf ]; then
    /usr/local/bin/neper generate-config > /etc/neper/neper.ini
fi

# Fix ownership and drop privileges if running as root
if [ "$(id -u)" = "0" ]; then
    echo "Root mode, fixing permissions and dropping privileges"
    # Modify neper user/group to match requested UID/GID
    deluser neper 2>/dev/null || true
    delgroup neper 2>/dev/null || true
    addgroup -g "$NEPER_GID" neper
    adduser -D -u "$NEPER_UID" -G neper neper

    chown -R neper:neper /etc/neper /home/neper
    exec su-exec neper /usr/local/bin/neper "$@"
else
    echo "non root mode, running as is"
    exec /usr/local/bin/neper "$@"
fi
