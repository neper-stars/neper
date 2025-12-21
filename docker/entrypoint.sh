#!/bin/bash

# Allow configurable UID/GID via environment variables
# Defaults to 1000:1000 if not set
NEPER_UID=${NEPER_UID:-1000}
NEPER_GID=${NEPER_GID:-1000}

# Fix ownership and drop privileges if running as root
if [ "$(id -u)" = "0" ]; then
    echo "Root mode, fixing permissions and dropping privileges"
    # Modify neper user/group to match requested UID/GID
    # Delete existing user/group if they exist
    userdel neper 2>/dev/null || true
    groupdel neper 2>/dev/null || true
    # Create group and user with specified IDs
    groupadd -g "$NEPER_GID" neper
    useradd -m -u "$NEPER_UID" -g neper -s /bin/bash neper

    chown -R neper:neper /home/neper
    exec gosu neper /usr/local/bin/neper "$@"
else
    echo "non root mode, running as is"
    exec /usr/local/bin/neper "$@"
fi
