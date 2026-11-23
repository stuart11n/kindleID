#!/bin/sh
# Shared Kindle UI takeover / restore (KOReader-style).
# Soft pillow-disable is not enough on modern FW — library/Awesome redraws over FBInk.

FBINK="/mnt/us/libkh/bin/fbink"
[ -x "$FBINK" ] || FBINK="fbink"

np_msg() {
	if [ -x "$FBINK" ] || command -v "$FBINK" >/dev/null 2>&1; then
		"$FBINK" -q -y 10 -x 1 "$1"
	else
		eips 1 10 "$1"
	fi
}

np_stop_framework() {
	lipc-set-prop com.lab126.powerd preventScreenSaver 1 >/dev/null 2>&1
	# Stop the Amazon GUI so nothing owns the framebuffer but us.
	if [ -f /etc/upstart/lab126_gui.conf ] || [ -f /etc/init/lab126_gui.conf ]; then
		# Avoid KUAL/scriptlet SIGTERM killing us while gui tears down.
		trap "" TERM
		stop lab126_gui >/dev/null 2>&1
		usleep 1250000 2>/dev/null || sleep 2
		trap - TERM
	elif [ -x /etc/init.d/framework ]; then
		/etc/init.d/framework stop >/dev/null 2>&1
		sleep 2
	else
		# Fallback: hard-disable pillow + freeze Awesome + kill chrome bars.
		lipc-set-prop com.lab126.pillow disableEnablePillow disable >/dev/null 2>&1
		killall -STOP awesome >/dev/null 2>&1
		for j in statusbar chrome; do
			if [ -f "/etc/upstart/${j}.conf" ] || [ -f "/etc/init/${j}.conf" ]; then
				stop "$j" >/dev/null 2>&1
			fi
		done
	fi
}

np_start_framework() {
	if [ -f /etc/upstart/lab126_gui.conf ] || [ -f /etc/init/lab126_gui.conf ]; then
		start lab126_gui >/dev/null 2>&1
	elif [ -x /etc/init.d/framework ]; then
		/etc/init.d/framework start >/dev/null 2>&1
	else
		killall -CONT awesome >/dev/null 2>&1
		lipc-set-prop com.lab126.pillow disableEnablePillow enable >/dev/null 2>&1
		for j in statusbar chrome; do
			if [ -f "/etc/upstart/${j}.conf" ] || [ -f "/etc/init/${j}.conf" ]; then
				start "$j" >/dev/null 2>&1
			fi
		done
	fi
	lipc-set-prop com.lab126.powerd preventScreenSaver 0 >/dev/null 2>&1
}
