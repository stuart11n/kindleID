package detect

import (
	"sync"
	"time"
)

// Config is the timing the listener uses to decide when a record started,
// when to ask Shazam again, and when the room has gone quiet.
type Config struct {
	SampleRate int
	Threshold  float64
	Settle     time.Duration
	Clip       time.Duration
	Gap        time.Duration
	Silence    time.Duration
	Recheck    time.Duration
}

// Action is what the listener should do after a chunk of audio.
type Action struct {
	Identify    bool
	Clip        []float32
	Generation  int
	Reason      string
	ShowNothing bool
}

type phase int

const (
	phaseIdle phase = iota
	phaseCollect
	phasePlaying
)

// Machine turns a mono stream into identify requests.
// A needle drop or the gap between tracks starts a new identification after
// Settle of sound, using Clip of audio. While a record plays, it asks again
// every Recheck. Silence for Silence shows "Nothing playing."
type Machine struct {
	cfg          Config
	settleN      int
	clipN        int
	gapN         int
	silenceN     int
	recheckN     int
	ring         *ring
	phase        phase
	silenceRun   int
	soundRun     int
	sinceRecheck int
	missFails    int // consecutive no-matches while still playing
	inFlight     bool
	haveTrack    bool
	nothingShown bool
	gen          int
	lastRMS      float64
	mu           sync.Mutex
}

func New(cfg Config) *Machine {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 16000
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 0.02
	}
	if cfg.Settle <= 0 {
		cfg.Settle = 6 * time.Second
	}
	if cfg.Clip <= 0 {
		cfg.Clip = 10 * time.Second
	}
	if cfg.Gap <= 0 {
		cfg.Gap = 2 * time.Second
	}
	if cfg.Silence <= 0 {
		cfg.Silence = 20 * time.Second
	}
	if cfg.Recheck <= 0 {
		cfg.Recheck = time.Minute
	}
	return &Machine{
		cfg:      cfg,
		settleN:  samples(cfg.SampleRate, cfg.Settle),
		clipN:    samples(cfg.SampleRate, cfg.Clip),
		gapN:     samples(cfg.SampleRate, cfg.Gap),
		silenceN: samples(cfg.SampleRate, cfg.Silence),
		recheckN: samples(cfg.SampleRate, cfg.Recheck),
		ring:     newRing(samples(cfg.SampleRate, cfg.Clip)),
	}
}

func samples(rate int, d time.Duration) int {
	n := int(float64(rate) * d.Seconds())
	if n < 1 {
		return 1
	}
	return n
}

// Push feeds the next chunk of mono audio.
func (m *Machine) Push(samples []float32) Action {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(samples) == 0 {
		return Action{}
	}
	m.ring.write(samples)
	m.lastRMS = rms(samples)
	n := len(samples)
	silent := m.lastRMS < m.cfg.Threshold

	if silent {
		m.silenceRun += n
	} else {
		m.silenceRun = 0
	}
	if m.silenceRun >= m.gapN {
		if m.phase != phaseIdle {
			m.phase = phaseIdle
			m.sinceRecheck = 0
			m.missFails = 0
		}
		m.soundRun = 0
	} else {
		m.soundRun += n
	}

	if m.silenceRun >= m.silenceN && !m.nothingShown {
		m.nothingShown = true
		m.haveTrack = false
		m.inFlight = false
		m.missFails = 0
		m.phase = phaseIdle
		m.gen++
		return Action{ShowNothing: true}
	}

	if m.phase == phaseIdle && m.soundRun >= m.settleN {
		m.phase = phaseCollect
	}
	if m.phase == phaseCollect && m.soundRun >= m.clipN && !m.inFlight {
		m.phase = phasePlaying
		m.sinceRecheck = 0
		return m.startIdentify("start")
	}
	if m.phase == phasePlaying && !m.inFlight {
		m.sinceRecheck += n
		if m.sinceRecheck >= m.recheckN && m.silenceRun == 0 {
			m.sinceRecheck = 0
			return m.startIdentify("recheck")
		}
	}
	return Action{}
}

func (m *Machine) startIdentify(reason string) Action {
	m.gen++
	m.inFlight = true
	return Action{
		Identify:   true,
		Clip:       m.ring.last(m.clipN),
		Generation: m.gen,
		Reason:     reason,
	}
}

// Finish closes an in-flight identification. accept is false when a newer
// event, usually "Nothing playing.", has already superseded this result.
func (m *Machine) Finish(gen int, found bool) (accept bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.gen {
		return false
	}
	m.inFlight = false
	if found {
		m.haveTrack = true
		m.nothingShown = false
		m.missFails = 0
		m.phase = phasePlaying
	} else if m.phase != phaseIdle {
		m.phase = phasePlaying
	}
	return true
}

// ScheduleRecheck sets how long to wait before the next playing-state identify.
// Call after a successful match using NextRecheck(...).
func (m *Machine) ScheduleRecheck(wait time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if wait <= 0 {
		wait = m.cfg.Recheck
	}
	m.missFails = 0
	m.recheckN = samples(m.cfg.SampleRate, wait)
	m.sinceRecheck = 0
}

// ScheduleMissRecheck backs off after a no-match: 10s first, then Recheck (60s).
func (m *Machine) ScheduleMissRecheck() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.missFails++
	wait := 10 * time.Second
	if m.missFails > 1 {
		wait = m.cfg.Recheck
		if wait <= 0 {
			wait = time.Minute
		}
	}
	m.recheckN = samples(m.cfg.SampleRate, wait)
	m.sinceRecheck = 0
	return wait
}

// RecheckEvery reports the current recheck interval.
func (m *Machine) RecheckEvery() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.SampleRate <= 0 {
		return m.cfg.Recheck
	}
	return time.Duration(float64(m.recheckN) / float64(m.cfg.SampleRate) * float64(time.Second))
}

func (m *Machine) State() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.inFlight && !m.haveTrack:
		return "identifying"
	case m.phase == phasePlaying:
		return "playing"
	case m.soundRun > 0:
		return "listening"
	default:
		return "idle"
	}
}

func (m *Machine) RMS() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRMS
}

func rms(s []float32) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		sum += float64(v) * float64(v)
	}
	return sqrt(sum / float64(len(s)))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 12; i++ {
		z = (z + x/z) / 2
	}
	return z
}

type ring struct {
	buf  []float32
	pos  int
	full bool
}

func newRing(n int) *ring {
	if n < 1 {
		n = 1
	}
	return &ring{buf: make([]float32, n)}
}

func (r *ring) write(s []float32) {
	for _, v := range s {
		r.buf[r.pos] = v
		r.pos++
		if r.pos == len(r.buf) {
			r.pos = 0
			r.full = true
		}
	}
}

func (r *ring) last(n int) []float32 {
	available := r.pos
	if r.full {
		available = len(r.buf)
	}
	if n > available {
		n = available
	}
	out := make([]float32, n)
	start := r.pos - n
	if start < 0 {
		start += len(r.buf)
	}
	for i := 0; i < n; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}
