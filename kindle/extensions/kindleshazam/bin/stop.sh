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

if [ -f "$PIDFILE" ]; then
	pid=$(cat "$PIDFILE")
	kill "$pid" 2>/dev/null
	sleep 1
	kill -9 "$pid" 2>/dev/null
	rm -f "$PIDFILE"
	msg "stopped ($pid)"
else
	msg "not running"
fi
lipc-set-prop com.lab126.powerd preventScreenSaver 0 >/dev/null 2>&1
if [ -f /etc/upstart/statusbar.conf ]; then
	start statusbar >/dev/null 2>&1
fi
lipc-set-prop com.lab126.pillow disableEnablePillow enable >/dev/null 2>&1
exit 0
