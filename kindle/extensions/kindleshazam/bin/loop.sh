#!/bin/sh
EXT="/mnt/us/extensions/kindleshazam"
STATE="/var/tmp/kindleshazam"
PIDFILE="/var/tmp/kindleshazam.pid"
FBINK="/mnt/us/libkh/bin/fbink"
if [ ! -x "$FBINK" ]; then
	FBINK="fbink"
fi
# shellcheck source=chrome.sh
. "$EXT/bin/chrome.sh"
mkdir -p "$STATE"
last=""
last_fl=""
n=0
fail=0
interval=10

msg() {
	np_msg "$1"
}

restore_and_exit() {
	echo "$(date) remote stop — restoring UI" >>"$EXT/loop.log"
	np_msg "restoring Kindle UI..."
	np_start_framework
	rm -f "$PIDFILE"
	exit 0
}

while true; do
	# Keep wifi/session alive; framework is already stopped by start.sh.
	lipc-set-prop com.lab126.powerd -i deferSuspend 120000 >/dev/null 2>&1
	lipc-set-prop com.lab126.powerd preventScreenSaver 1 >/dev/null 2>&1
	base=$(tr -d '[:space:]' < "$EXT/server.url")
	if [ -z "$base" ] || [ "$base" = "http://PI_HOST:8080" ]; then
		msg "bad server.url"
		echo "$(date) bad server.url '$base'" >>"$EXT/loop.log"
		sleep 10
		continue
	fi
	if wget -q -U KindleShazam/1 -O "$STATE/version.new" "$base/version"; then
		fail=0
		ver=$(sed -n '1p' "$STATE/version.new" | tr -d '[:space:]')
		next=$(sed -n '2p' "$STATE/version.new" | tr -d '[:space:]')
		fl=$(sed -n '3p' "$STATE/version.new" | tr -d '[:space:]')
		cmd=$(sed -n '4p' "$STATE/version.new" | tr -d '[:space:]')
		if [ "$cmd" = "stop" ]; then
			restore_and_exit
		fi
		case "$next" in
			2|10) interval=$next ;;
			*) interval=10 ;;
		esac
		# Apply frontlight when server requests a level (0–24).
		case "$fl" in
			''|'-') ;;
			*[!0-9]*) ;;
			*)
				if [ "$fl" -ge 0 ] 2>/dev/null && [ "$fl" -le 24 ] 2>/dev/null; then
					if [ "$fl" != "$last_fl" ]; then
						if lipc-set-prop -i com.lab126.powerd flIntensity "$fl" >/dev/null 2>&1; then
							last_fl="$fl"
							echo "$(date) frontlight $fl" >>"$EXT/loop.log"
						fi
					fi
				fi
				;;
		esac
		if [ -n "$ver" ] && [ "$ver" != "$last" ]; then
			if wget -q -U KindleShazam/1 -O "$STATE/screen.png" "$base/screen.png"; then
				n=$((n + 1))
				if [ $((n % 10)) -eq 1 ]; then
					"$FBINK" -g "file=$STATE/screen.png" -W GC16 -f
					echo "$(date) flash redraw $n $ver" >>"$EXT/loop.log"
				else
					"$FBINK" -g "file=$STATE/screen.png" -W GL16
					echo "$(date) redraw $n $ver" >>"$EXT/loop.log"
				fi
				last="$ver"
			else
				echo "$(date) screen fetch failed" >>"$EXT/loop.log"
				msg "screen fetch failed"
			fi
		fi
	else
		fail=$((fail + 1))
		echo "$(date) version fetch failed ($base)" >>"$EXT/loop.log"
		if [ "$fail" -le 3 ] || [ $((fail % 6)) -eq 0 ]; then
			msg "no server: $base"
		fi
		interval=10
	fi
	sleep "$interval"
done
