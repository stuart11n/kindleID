package listen

import (
	"testing"
	"time"
)

func TestSolarQuietHours(t *testing.T) {
	loc := time.Local
	cases := []struct {
		hour int
		want bool
	}{
		{0, true}, {5, true}, {6, false}, {12, false}, {20, false}, {21, true}, {23, true},
	}
	for _, c := range cases {
		now := time.Date(2026, 11, 25, c.hour, 30, 0, 0, loc)
		if got := solarQuietHours(now); got != c.want {
			t.Fatalf("hour %d: quiet=%v want %v", c.hour, got, c.want)
		}
	}
}

func TestNextSolarDelayQuiet(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 11, 25, 22, 15, 0, 0, loc)
	d := nextSolarDelay(now, time.Hour)
	wake := now.Add(d)
	if wake.Hour() != 6 || wake.Minute() != 0 || wake.Day() != 26 {
		t.Fatalf("wake %v want tomorrow 06:00", wake)
	}

	now = time.Date(2026, 11, 25, 3, 0, 0, 0, loc)
	d = nextSolarDelay(now, time.Hour)
	wake = now.Add(d)
	if wake.Hour() != 6 || wake.Day() != 25 {
		t.Fatalf("wake %v want today 06:00", wake)
	}

	now = time.Date(2026, 11, 25, 10, 0, 0, 0, loc)
	if d = nextSolarDelay(now, time.Hour); d != time.Hour {
		t.Fatalf("daytime delay %v want 1h", d)
	}
}
