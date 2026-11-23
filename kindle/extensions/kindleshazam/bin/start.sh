#!/bin/sh
EXT="/mnt/us/extensions/kindleshazam"
PIDFILE="/var/tmp/kindleshazam.pid"
FBINK="/mnt/us/libkh/bin/fbink"
[ -x "$FBINK" ] || FBINK="fbink"
msg() {
	if [ -x "$FBINK" ] || command -v "$FBINK" >/dev/null 2>&1; then
		"$FBINK" -q -y 10 -x 1 "$1"
	else
		eips 1 10 "$1"
	fi
}

mkdir -p /var/tmp/kindleshazam
if [ -f "$PIDFILE" ]; then
	pid=$(cat "$PIDFILE")
	if kill -0 "$pid" 2>/dev/null; then
		msg "already running ($pid)"
		exit 0
	fi
fi

url=$(tr -d '[:space:]' < "$EXT/server.url" 2>/dev/null)
case "$url" in
""|http://PI_HOST:8080)
	msg "fix: set server.url first"
	exit 1
	;;
esac

lipc-set-prop com.lab126.powerd preventScreenSaver 1 >/dev/null 2>&1
lipc-set-prop com.lab126.pillow disableEnablePillow disable >/dev/null 2>&1
if [ -f /etc/upstart/statusbar.conf ]; then
	stop statusbar >/dev/null 2>&1
fi

: >"$EXT/loop.log"
msg "starting -> $url"
sh "$EXT/bin/loop.sh" >>"$EXT/loop.log" 2>&1 &
echo $! >"$PIDFILE"
sleep 2
if kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
	msg "running; polling..."
else
	msg "start failed; see loop.log"
	exit 1
fi
exit 0
