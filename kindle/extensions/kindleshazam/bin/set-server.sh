#!/bin/sh
# Usage: set-server.sh <url>
EXT="/mnt/us/extensions/kindleshazam"
FBINK="/mnt/us/libkh/bin/fbink"
[ -x "$FBINK" ] || FBINK="fbink"
url=$1
if [ -z "$url" ] || [ "$url" = "http://PI_HOST:8080" ]; then
	msg="set-server: bad/placeholder url"
else
	printf '%s\n' "$url" >"$EXT/server.url"
	msg="server.url set"
fi
if command -v "$FBINK" >/dev/null 2>&1 || [ -x "$FBINK" ]; then
	"$FBINK" -q -y 10 -x 1 "$msg"
	"$FBINK" -q -y 12 -x 1 "$url"
else
	eips 1 10 "$msg"
	eips 1 12 "$url"
fi
exit 0
