#!/bin/sh
EXT="/mnt/us/extensions/kindleshazam"
PIDFILE="/var/tmp/kindleshazam.pid"
# shellcheck source=chrome.sh
. "$EXT/bin/chrome.sh"

mkdir -p /var/tmp/kindleshazam
if [ -f "$PIDFILE" ]; then
	pid=$(cat "$PIDFILE")
	if kill -0 "$pid" 2>/dev/null; then
		np_msg "already running ($pid)"
		exit 0
	fi
fi

url=$(tr -d '[:space:]' < "$EXT/server.url" 2>/dev/null)
case "$url" in
""|http://PI_HOST:8080)
	np_msg "fix: set server.url first"
	exit 1
	;;
esac

: >"$EXT/loop.log"
# Clear any remote-stop latch on the Pi so a fresh Start keeps running.
wget -q -O /dev/null "$url/kindle/clear" >/dev/null 2>&1 || true
np_msg "taking over screen..."
np_stop_framework
np_msg "starting -> $url"
sh "$EXT/bin/loop.sh" >>"$EXT/loop.log" 2>&1 &
echo $! >"$PIDFILE"
sleep 2
if ! kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
	np_msg "start failed; restoring UI"
	np_start_framework
	exit 1
fi
exit 0
