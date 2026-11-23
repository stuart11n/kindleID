package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
)

// Meta is the track currently on the screen.
type Meta struct {
	Key    string
	Title  string
	Artist string
	Album  string
	Year   string
}

// Status is the JSON the machine reports while you are testing.
type Status struct {
	State     string  `json:"state"`
	RMS       float64 `json:"rms"`
	Threshold float64 `json:"threshold"`
	Device    string  `json:"device"`
	Version   string  `json:"version"`
	Title     string  `json:"title,omitempty"`
	Artist    string  `json:"artist,omitempty"`
	Album     string  `json:"album,omitempty"`
	Year      string  `json:"year,omitempty"`
}

// Display holds the PNG and the tiny version the Kindle polls.
type Display struct {
	mu        sync.RWMutex
	png       []byte
	version   string
	state     string
	rms       float64
	threshold float64
	device    string
	meta      Meta
	logs      *LogBuf
}

func NewDisplay(device string, threshold float64) *Display {
	return &Display{
		state:     "idle",
		device:    device,
		threshold: threshold,
		version:   "0",
		logs:      NewLogBuf(50),
	}
}

// Logs is the rolling buffer mirrored to /log.
func (d *Display) Logs() *LogBuf {
	return d.logs
}

// Set publishes a new screen. The version changes only when the pixels do,
// so a recheck of the same record does not make the Kindle redraw.
func (d *Display) Set(png []byte, meta Meta) bool {
	sum := sha256.Sum256(png)
	version := hex.EncodeToString(sum[:8])
	d.mu.Lock()
	defer d.mu.Unlock()
	if version == d.version {
		return false
	}
	d.png = append([]byte(nil), png...)
	d.version = version
	d.meta = meta
	return true
}

// SameTrack reports whether key is already on screen.
func (d *Display) SameTrack(key string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return key != "" && key == d.meta.Key
}

// Idle reports whether the screen is showing weather / nothing-playing.
func (d *Display) Idle() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.meta.Key == "" && d.meta.Title == ""
}

// SetDevice records the capture endpoint that was actually opened.
func (d *Display) SetDevice(name string) {
	d.mu.Lock()
	d.device = name
	d.mu.Unlock()
}

// Observe updates the live meter without touching the picture.
func (d *Display) Observe(state string, rms float64) {
	d.mu.Lock()
	d.state = state
	d.rms = rms
	d.mu.Unlock()
}

func (d *Display) snapshot() (png []byte, version string, status Status) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	png = append([]byte(nil), d.png...)
	version = d.version
	status = Status{
		State:     d.state,
		RMS:       d.rms,
		Threshold: d.threshold,
		Device:    d.device,
		Version:   d.version,
		Title:     d.meta.Title,
		Artist:    d.meta.Artist,
		Album:     d.meta.Album,
		Year:      d.meta.Year,
	}
	return png, version, status
}

// Handler serves /version, /screen.png, /status, and /log.
func (d *Display) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_, version, _ := d.snapshot()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(version + "\n"))
	})
	mux.HandleFunc("/screen.png", func(w http.ResponseWriter, r *http.Request) {
		png, _, _ := d.snapshot()
		if len(png) == 0 {
			http.Error(w, "no screen yet", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		_, _, status := d.snapshot()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(status)
	})
	mux.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if d.logs == nil {
			return
		}
		const tailN = 50
		follow := true
		if v := r.URL.Query().Get("follow"); v == "0" || v == "false" {
			follow = false
		}
		if !follow {
			chunk, _ := d.logs.TailLines(tailN)
			_, _ = w.Write(chunk)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			chunk, _ := d.logs.TailLines(tailN)
			_, _ = w.Write(chunk)
			return
		}
		w.Header().Set("Transfer-Encoding", "chunked")
		_ = d.logs.Follow(r.Context(), tailN, func(p []byte) error {
			if _, err := w.Write(p); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
	})
	return mux
}
