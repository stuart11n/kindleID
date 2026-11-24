package identify

import (
	"sync"
)

// Stats tracks per-provider identify outcomes for A/B display.
type Stats struct {
	mu       sync.Mutex
	s        map[string]*provStat
	onChange func()
}

type provStat struct {
	attempts int
	fails    int
	wins     int // first-to-answer wins
}

func newStats() *Stats {
	return &Stats{s: map[string]*provStat{
		"shazam": {},
		"audd":   {},
	}}
}

// OnChange registers a hook invoked after each recorded attempt (e.g. refresh footer).
func (st *Stats) OnChange(fn func()) {
	if st == nil {
		return
	}
	st.mu.Lock()
	st.onChange = fn
	st.mu.Unlock()
}

func (st *Stats) notify() {
	st.mu.Lock()
	fn := st.onChange
	st.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// RecordAttempt notes whether a provider matched this clip.
func (st *Stats) RecordAttempt(provider string, matched bool) {
	if st == nil {
		return
	}
	st.mu.Lock()
	p := st.s[provider]
	if p == nil {
		p = &provStat{}
		st.s[provider] = p
	}
	p.attempts++
	if !matched {
		p.fails++
	}
	st.mu.Unlock()
	st.notify()
}

// RecordWin notes which provider answered first with a match.
func (st *Stats) RecordWin(provider string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	p := st.s[provider]
	if p == nil {
		p = &provStat{}
		st.s[provider] = p
	}
	p.wins++
	st.mu.Unlock()
}

// Footer used to show A/B fail rates; kept empty so the Kindle stays clean.
func (st *Stats) Footer() string {
	return ""
}
