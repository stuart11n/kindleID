package identify

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// raceRecognizer runs Shazam and AudD in parallel; first match wins the screen.
// Both attempts are always recorded for the S/A failure-rate footer.
type raceRecognizer struct {
	shazam recognizer
	audd   recognizer
	stats  *Stats
	seq    atomic.Uint64
}

type raceOut struct {
	id      uint64
	name    string
	track   *Track
	err     error
	elapsed time.Duration
}

func (r *raceRecognizer) Identify(ctx context.Context, samples []float32) (*Track, error) {
	id := r.seq.Add(1)
	log.Printf("ab #%d: race start (%ds clip)", id, len(samples)/16000)

	// Detach so returning a fast winner (and canceling the parent) does not
	// abort the loser before we can record its result.
	base := context.WithoutCancel(ctx)
	ch := make(chan raceOut, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	launch := func(name string, rec recognizer) {
		defer wg.Done()
		r.run(base, id, name, rec, samples, ch)
	}
	go launch("shazam", r.shazam)
	go launch("audd", r.audd)
	go func() {
		wg.Wait()
		close(ch)
	}()

	gotWinner := make(chan *Track, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Must be a flag — after Identify receives from gotWinner the buffer is
		// empty again, so a second send would wrongly look like a first win.
		var haveWinner bool
		for o := range ch {
			matched := o.err == nil && o.track != nil
			r.stats.RecordAttempt(o.name, matched)
			logAnswer(o)
			if !matched {
				continue
			}
			if haveWinner {
				log.Printf("ab #%d: %s slower hit (not used)", id, o.name)
				continue
			}
			haveWinner = true
			r.stats.RecordWin(o.name)
			log.Printf("ab #%d: winner %s", id, o.name)
			gotWinner <- o.track
		}
	}()

	select {
	case w := <-gotWinner:
		return w, nil
	case <-done:
		select {
		case w := <-gotWinner:
			return w, nil
		default:
			log.Printf("ab #%d: both missed", id)
			return nil, nil
		}
	case <-ctx.Done():
		select {
		case w := <-gotWinner:
			return w, nil
		default:
			return nil, ctx.Err()
		}
	}
}

func logAnswer(o raceOut) {
	ms := o.elapsed.Round(time.Millisecond)
	switch {
	case o.err != nil:
		log.Printf("ab #%d: %s error after %s: %v", o.id, o.name, ms, o.err)
	case o.track != nil:
		log.Printf("ab #%d: %s → %s — %s (%s)", o.id, o.name, o.track.Artist, o.track.Title, ms)
	default:
		log.Printf("ab #%d: %s → no match (%s)", o.id, o.name, ms)
	}
}

func (r *raceRecognizer) run(base context.Context, id uint64, name string, rec recognizer, samples []float32, ch chan<- raceOut) {
	defer func() {
		if v := recover(); v != nil {
			ch <- raceOut{id: id, name: name, err: fmt.Errorf("panic: %v", v)}
		}
	}()
	log.Printf("ab #%d: %s querying…", id, name)
	// Shazam fingerprinting on Pi 1 can still drag; don't let it sit for 45s.
	limit := 12 * time.Second
	if name == "audd" {
		limit = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(base, limit)
	defer cancel()
	start := time.Now()
	track, err := rec.Identify(ctx, samples)
	elapsed := time.Since(start)
	if track != nil {
		cp := *track
		cp.Provider = name
		track = &cp
	}
	ch <- raceOut{id: id, name: name, track: track, err: err, elapsed: elapsed}
}
