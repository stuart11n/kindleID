package detect

import "time"

// NextRecheck returns when to ask Shazam again after a match.
// offset is where in the track the clip matched; duration is the track length.
// Worst case waits until the digital track should be over, plus margin for vinyl drift.
func NextRecheck(duration, offset, clip, fallback, margin time.Duration) time.Duration {
	if duration <= 0 || offset < 0 {
		return fallback
	}
	// Position at end of our clip (worst case: still in this track until then).
	pos := offset + clip
	if pos < offset {
		pos = offset
	}
	remaining := duration - pos
	if remaining < 0 {
		remaining = 0
	}
	wait := remaining + margin
	const minWait = 20 * time.Second
	const maxWait = 3 * time.Minute
	if wait < minWait {
		wait = minWait
	}
	if wait > maxWait {
		wait = maxWait
	}
	return wait
}
