#!/bin/sh
EXT="/mnt/us/extensions/kindleshazam"
PIDFILE="/var/tmp/kindleshazam.pid"
# shellcheck source=chrome.sh
. "$EXT/bin/chrome.sh"

if [ -f "$PIDFILE" ]; then
	pid=$(cat "$PIDFILE")
	kill "$pid" 2>/dev/null
	sleep 1
	kill -9 "$pid" 2>/dev/null
	rm -f "$PIDFILE"
	np_msg "stopped ($pid)"
else
	np_msg "not running"
fi

np_msg "restoring Kindle UI..."
np_start_framework
sleep 2
np_msg "UI restored"
exit 0
