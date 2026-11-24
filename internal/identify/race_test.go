package identify

import (
	"context"
	"testing"
	"time"
)

type slowRec struct {
	delay time.Duration
	track *Track
}

func (s slowRec) Identify(ctx context.Context, samples []float32) (*Track, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
		return s.track, nil
	}
}

func TestRaceFirstMatchWins(t *testing.T) {
	stats := newStats()
	r := &raceRecognizer{
		shazam: slowRec{delay: 80 * time.Millisecond, track: &Track{Title: "S", Artist: "A"}},
		audd:   slowRec{delay: 10 * time.Millisecond, track: &Track{Title: "A", Artist: "B"}},
		stats:  stats,
	}
	start := time.Now()
	track, err := r.Identify(context.Background(), make([]float32, 16000))
	if err != nil || track == nil {
		t.Fatalf("track=%v err=%v", track, err)
	}
	if track.Provider != "audd" || track.Title != "A" {
		t.Fatalf("got %+v", track)
	}
	if time.Since(start) > 60*time.Millisecond {
		t.Fatalf("did not return on first answer: %s", time.Since(start))
	}
	time.Sleep(120 * time.Millisecond) // let loser record
	if s, a := stats.s["shazam"], stats.s["audd"]; s == nil || a == nil || s.attempts != 1 || a.attempts != 1 || s.fails != 0 || a.fails != 0 {
		t.Fatalf("shazam=%+v audd=%+v", s, a)
	}
}

func TestRaceRecordsMiss(t *testing.T) {
	stats := newStats()
	r := &raceRecognizer{
		shazam: slowRec{delay: 5 * time.Millisecond, track: nil},
		audd:   slowRec{delay: 5 * time.Millisecond, track: &Track{Title: "X", Artist: "Y"}},
		stats:  stats,
	}
	track, err := r.Identify(context.Background(), make([]float32, 16000))
	if err != nil || track == nil || track.Provider != "audd" {
		t.Fatalf("track=%+v err=%v", track, err)
	}
	time.Sleep(30 * time.Millisecond)
	if s := stats.s["shazam"]; s == nil || s.attempts != 1 || s.fails != 1 {
		t.Fatalf("shazam miss not recorded: %+v", s)
	}
	if a := stats.s["audd"]; a == nil || a.attempts != 1 || a.fails != 0 {
		t.Fatalf("audd=%+v", a)
	}
}

func TestRaceLoserRecordedAfterEarlyReturn(t *testing.T) {
	stats := newStats()
	changed := make(chan struct{}, 4)
	stats.OnChange(func() { changed <- struct{}{} })
	r := &raceRecognizer{
		shazam: slowRec{delay: 80 * time.Millisecond, track: nil},
		audd:   slowRec{delay: 5 * time.Millisecond, track: &Track{Title: "Fast", Artist: "A"}},
		stats:  stats,
	}
	start := time.Now()
	track, err := r.Identify(context.Background(), make([]float32, 16000))
	if err != nil || track == nil || track.Provider != "audd" {
		t.Fatalf("track=%+v err=%v", track, err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("did not return early: %s", time.Since(start))
	}
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case <-changed:
			if s := stats.s["shazam"]; s != nil && s.attempts == 1 && s.fails == 1 {
				return
			}
		case <-deadline:
			t.Fatalf("loser never recorded; shazam=%+v", stats.s["shazam"])
		}
	}
}

func TestRaceOnlyOneWinnerWhenBothHit(t *testing.T) {
	stats := newStats()
	r := &raceRecognizer{
		shazam: slowRec{delay: 40 * time.Millisecond, track: &Track{Title: "Slow", Artist: "S"}},
		audd:   slowRec{delay: 5 * time.Millisecond, track: &Track{Title: "Fast", Artist: "A"}},
		stats:  stats,
	}
	track, err := r.Identify(context.Background(), make([]float32, 16000))
	if err != nil || track == nil || track.Provider != "audd" {
		t.Fatalf("track=%+v err=%v", track, err)
	}
	time.Sleep(80 * time.Millisecond)
	st := stats.s["audd"]
	if st == nil || st.wins != 1 {
		t.Fatalf("audd wins=%v want 1", st)
	}
	if w := stats.s["shazam"]; w == nil || w.wins != 0 || w.attempts != 1 {
		t.Fatalf("shazam stats=%+v", w)
	}
}
