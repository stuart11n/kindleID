#!/bin/sh
EXT="/mnt/us/extensions/kindleshazam"
STATE="/var/tmp/kindleshazam"
FBINK="/mnt/us/libkh/bin/fbink"
if [ ! -x "$FBINK" ]; then
	FBINK="fbink"
fi
mkdir -p "$STATE"
last=""
n=0
fail=0

msg() {
	if [ -x "$FBINK" ] || command -v "$FBINK" >/dev/null 2>&1; then
		"$FBINK" -q -y 10 -x 1 "$1"
	else
		eips 1 10 "$1"
	fi
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
	if wget -q -O "$STATE/version.new" "$base/version"; then
		fail=0
		ver=$(tr -d '[:space:]' < "$STATE/version.new")
		if [ -n "$ver" ] && [ "$ver" != "$last" ]; then
			if wget -q -O "$STATE/screen.png" "$base/screen.png"; then
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
	fi
	sleep 10
done
