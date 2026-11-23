#!/bin/sh
EXT="/mnt/us/extensions/kindleshazam"
FBINK="/mnt/us/libkh/bin/fbink"
[ -x "$FBINK" ] || FBINK="fbink"
url=$(tr -d '[:space:]' < "$EXT/server.url" 2>/dev/null)
[ -n "$url" ] || url="(no server.url)"
if command -v "$FBINK" >/dev/null 2>&1 || [ -x "$FBINK" ]; then
	"$FBINK" -q -y 10 -x 1 "server.url:"
	"$FBINK" -q -y 12 -x 1 "$url"
else
	eips 1 10 "server.url:"
	eips 1 12 "$url"
fi
exit 0
