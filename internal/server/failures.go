package server

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxMatchFailures = 10

// MatchFailure is one Shazam miss kept in RAM as a WAV.
type MatchFailure struct {
	ID        int       `json:"id"`
	At        time.Time `json:"at"`
	Reason    string    `json:"reason"`
	Rate      int       `json:"sample_rate"`
	Samples   int       `json:"samples"`
	Seconds   float64   `json:"seconds"`
	Bytes     int       `json:"bytes"`
	URL       string    `json:"url"`
	wav       []byte
}

// FailureStore keeps the last N match-failure WAVs in memory.
type FailureStore struct {
	mu   sync.Mutex
	next int
	ring []MatchFailure // newest last; max maxMatchFailures
}

func NewFailureStore() *FailureStore {
	return &FailureStore{}
}

// Add stores a mono float32 clip as WAV. Oldest entries drop off after 10.
func (s *FailureStore) Add(samples []float32, rate int, reason string) {
	if s == nil || len(samples) == 0 {
		return
	}
	if rate <= 0 {
		rate = 16000
	}
	wav := encodeWAV(samples, rate)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := s.next
	entry := MatchFailure{
		ID:      id,
		At:      time.Now().UTC(),
		Reason:  reason,
		Rate:    rate,
		Samples: len(samples),
		Seconds: float64(len(samples)) / float64(rate),
		Bytes:   len(wav),
		URL:     fmt.Sprintf("/match-failures/%d.wav", id),
		wav:     wav,
	}
	s.ring = append(s.ring, entry)
	if len(s.ring) > maxMatchFailures {
		s.ring = append([]MatchFailure(nil), s.ring[len(s.ring)-maxMatchFailures:]...)
	}
}

func (s *FailureStore) list() []MatchFailure {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]MatchFailure, len(s.ring))
	// newest first
	for i := range s.ring {
		out[len(s.ring)-1-i] = s.ring[i]
		out[len(s.ring)-1-i].wav = nil
	}
	return out
}

func (s *FailureStore) get(id int) (MatchFailure, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.ring {
		if e.ID == id {
			cp := e
			cp.wav = append([]byte(nil), e.wav...)
			return cp, true
		}
	}
	return MatchFailure{}, false
}

func encodeWAV(samples []float32, rate int) []byte {
	n := len(samples)
	dataBytes := n * 2
	buf := make([]byte, 44+dataBytes)
	// RIFF header
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataBytes))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(buf[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:], 1)  // mono
	binary.LittleEndian.PutUint32(buf[24:], uint32(rate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(rate*2)) // byte rate
	binary.LittleEndian.PutUint16(buf[32:], 2)              // block align
	binary.LittleEndian.PutUint16(buf[34:], 16)             // bits
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataBytes))
	for i, v := range samples {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(int16(v*32767)))
	}
	return buf
}

func (d *Display) Failures() *FailureStore {
	return d.failures
}

func (d *Display) handleMatchFailures(w http.ResponseWriter, r *http.Request) {
	if d.failures == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/match-failures")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		list := d.failures.list()
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") || r.URL.Query().Get("format") == "json" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"count":    len(list),
				"max":      maxMatchFailures,
				"failures": list,
			})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>match failures</title>")
		fmt.Fprintf(w, "<h1>Last %d Shazam match failures</h1>", maxMatchFailures)
		fmt.Fprintf(w, "<p>%d stored (RAM). <a href=\"/match-failures?format=json\">JSON</a></p><ol>", len(list))
		if len(list) == 0 {
			fmt.Fprintf(w, "<li>(none yet)</li>")
		}
		for _, e := range list {
			fmt.Fprintf(w, "<li><a href=\"%s\">fail-%d.wav</a> — %s — %s — %.1fs — %d bytes</li>",
				e.URL, e.ID, e.At.Format(time.RFC3339), e.Reason, e.Seconds, e.Bytes)
		}
		fmt.Fprintf(w, "</ol>")
		return
	}
	name := strings.TrimSuffix(path, ".wav")
	id, err := strconv.Atoi(name)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	e, ok := d.failures.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"match-fail-%d-%s.wav\"", e.ID, e.Reason))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(e.wav)))
	_, _ = w.Write(e.wav)
}
