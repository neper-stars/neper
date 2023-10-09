#!/bin/sh

/usr/local/bin/neper migrate -c /etc/neper/neper.conf && exec /usr/local/bin/neper serve -c /etc/neper/neper.conf
