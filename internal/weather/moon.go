package weather

import (
	"math"
	"time"
)

// MoonPhase returns illumination fraction 0..1 and a short name for t.
// 0 = new, 0.25 = first quarter, 0.5 = full, 0.75 = last quarter.
func MoonPhase(t time.Time) (frac float64, name string) {
	if t.IsZero() {
		t = time.Now()
	}
	// Synodic month from a known new moon: 2000-01-06 18:14 UTC.
	known := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	age := t.UTC().Sub(known).Hours() / 24
	const synodic = 29.530588853
	cycle := math.Mod(age, synodic)
	if cycle < 0 {
		cycle += synodic
	}
	frac = cycle / synodic
	switch {
	case frac < 0.03 || frac >= 0.97:
		name = "New moon"
	case frac < 0.22:
		name = "Waxing crescent"
	case frac < 0.28:
		name = "First quarter"
	case frac < 0.47:
		name = "Waxing gibbous"
	case frac < 0.53:
		name = "Full moon"
	case frac < 0.72:
		name = "Waning gibbous"
	case frac < 0.78:
		name = "Last quarter"
	default:
		name = "Waning crescent"
	}
	return frac, name
}
