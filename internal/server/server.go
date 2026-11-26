package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"kindle-shazam/internal/render"
	"kindle-shazam/internal/solaredge"
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
	covers    *artCache
	logs      *LogBuf
	failures       *FailureStore
	settings       *Settings
	lastMusicAt    time.Time // last time audio looked like music / a track was shown
	kindleStop     bool      // Kindle loop should restore UI and exit
	solarOAuth     *solaredge.OAuthClient
	solarPendingMu sync.Mutex
	solarPending   solarOAuthPending
}

func NewDisplay(device string, threshold float64) *Display {
	return &Display{
		state:     "idle",
		device:    device,
		threshold: threshold,
		version:   "0",
		covers:    newArtCache(defaultArtCacheBytes),
		logs:      NewLogBuf(50),
		failures:  NewFailureStore(),
	}
}

// Logs is the rolling buffer mirrored to /log.
func (d *Display) Logs() *LogBuf {
	return d.logs
}

// SetSettings attaches the runtime settings store (/settings).
func (d *Display) SetSettings(s *Settings) {
	d.mu.Lock()
	d.settings = s
	d.mu.Unlock()
}

// Settings returns the runtime settings store, if any.
func (d *Display) Settings() *Settings {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.settings
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
	if meta.Key != "" || meta.Title != "" {
		d.lastMusicAt = time.Now()
	}
	return true
}

// SameTrack reports whether key is already on screen.
func (d *Display) SameTrack(key string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return key != "" && key == d.meta.Key
}

// CoverArt returns a cached cover for artist/album or art URL, if present.
func (d *Display) CoverArt(artist, album, artURL string) image.Image {
	return d.covers.get(artist, album, artURL)
}

// RememberCover stores cover art in the 50 MiB FIFO cache (album + URL keys).
func (d *Display) RememberCover(artist, album, artURL string, art image.Image) {
	d.covers.put(artist, album, artURL, art)
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
	switch state {
	case "playing", "identifying", "listening":
		d.lastMusicAt = time.Now()
	}
	d.mu.Unlock()
}

// PollIntervalSeconds is how often the Kindle should hit /version.
// 2s while music has been seen in the last hour, otherwise 10s.
func (d *Display) PollIntervalSeconds() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.lastMusicAt.IsZero() && time.Since(d.lastMusicAt) < time.Hour {
		return 2
	}
	return 10
}

// RequestKindleStop asks the device loop to restore Amazon UI and exit.
func (d *Display) RequestKindleStop() {
	d.mu.Lock()
	d.kindleStop = true
	d.mu.Unlock()
}

// ClearKindleStop clears a pending stop (e.g. after Start from KUAL).
func (d *Display) ClearKindleStop() {
	d.mu.Lock()
	d.kindleStop = false
	d.mu.Unlock()
}

// KindleShouldStop reports whether the device loop should exit.
func (d *Display) KindleShouldStop() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.kindleStop
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

// Handler serves /version, /screen.png, /status, /log, /settings, and /match-failures.
func (d *Display) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/settings", d.handleSettings)
	mux.HandleFunc("/settings.json", d.handleSettings)
	mux.HandleFunc("/settings/restart", d.handleSettingsRestart)
	mux.HandleFunc("/settings/solaredge.json", d.handleSolarEdge)
	mux.HandleFunc("/settings/solaredge/authorize", d.handleSolarEdge)
	mux.HandleFunc("/settings/solaredge/start", d.handleSolarEdge)
	mux.HandleFunc("/settings/solaredge/callback", d.handleSolarEdge)
	mux.HandleFunc("/kindle/stop", d.handleKindleStop)
	mux.HandleFunc("/kindle/clear", d.handleKindleClear)
	mux.HandleFunc("/match-failures", d.handleMatchFailures)
	mux.HandleFunc("/match-failures/", d.handleMatchFailures)
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_, version, _ := d.snapshot()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		// Line 1 = screen version; line 2 = poll interval seconds (2 or 10);
		// line 3 = frontlight 0–24, or "-" to leave alone;
		// line 4 = "stop" to restore Kindle UI, else "run".
		fl := "-"
		if d.settings != nil {
			if n := d.settings.FrontlightLevel(); n >= 0 {
				fl = strconv.Itoa(n)
			}
		}
		cmd := "run"
		if d.KindleShouldStop() {
			cmd = "stop"
		}
		_, _ = w.Write([]byte(version + "\n" + strconv.Itoa(d.PollIntervalSeconds()) + "\n" + fl + "\n" + cmd + "\n"))
	})
	mux.HandleFunc("/screen.png", func(w http.ResponseWriter, r *http.Request) {
		png, _, _ := d.snapshot()
		if len(png) == 0 {
			http.Error(w, "no screen yet", http.StatusServiceUnavailable)
			return
		}
		// Stored PNG is soft-rotated CCW for FBInk. Browsers get CW so it reads upright.
		if !isKindle(r.UserAgent()) {
			if rotated, err := render.RotatePNGCW(png); err == nil {
				png = rotated
			}
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

func isKindle(ua string) bool {
	return strings.Contains(ua, "KindleShazam") || strings.Contains(ua, "Wget")
}
