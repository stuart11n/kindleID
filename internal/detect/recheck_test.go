package detect

import (
	"testing"
	"time"
)

func TestNextRecheckUsesRemaining(t *testing.T) {
	// 4:00 track, matched at 1:00, 10s clip → remaining 2:50 + 15s margin = 3:05 → capped at 3m
	wait := NextRecheck(4*time.Minute, time.Minute, 10*time.Second, time.Minute, 15*time.Second)
	if wait != 3*time.Minute {
		t.Fatalf("got %v want 3m", wait)
	}
}

func TestNextRecheckNearEnd(t *testing.T) {
	wait := NextRecheck(3*time.Minute, 2*time.Minute+50*time.Second, 10*time.Second, time.Minute, 15*time.Second)
	// remaining = 180 - 170 - 10 = 0 + 15s → floor 20s
	if wait != 20*time.Second {
		t.Fatalf("got %v want 20s", wait)
	}
}

func TestNextRecheckFallbackWithoutDuration(t *testing.T) {
	wait := NextRecheck(0, 30*time.Second, 10*time.Second, time.Minute, 15*time.Second)
	if wait != time.Minute {
		t.Fatalf("got %v want 1m", wait)
	}
}
