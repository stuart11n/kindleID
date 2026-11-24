package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SettingsValues are the tunables exposed on /settings.
type SettingsValues struct {
	Identify     string  `json:"identify"`
	Threshold    float64 `json:"threshold"`
	Settle       string  `json:"settle"`
	Clip         string  `json:"clip"`
	MissWait     string  `json:"miss_wait"`
	FullClip     string  `json:"full_clip"`
	Gap          string  `json:"gap"`
	Silence      string  `json:"silence"`
	Recheck      string  `json:"recheck"`
	WeatherEvery string  `json:"weather_every"`
	SolarEvery   string  `json:"solar_every"`
	Celsius      bool    `json:"celsius"`
	Radar        bool    `json:"radar"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	Place        string  `json:"place"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	// Frontlight is Kindle flIntensity 0–24, or -1 to leave the device alone.
	Frontlight int `json:"frontlight"`
	// AudDToken is write-only from the form; never persisted in GET JSON as plain
	// unless already stored. Stored in settings file for restarts.
	AudDToken string `json:"audd_api_token,omitempty"`
}

// Settings holds live values, persists to disk, and notifies listeners on save.
type Settings struct {
	mu       sync.RWMutex
	path     string
	values   SettingsValues
	hasToken bool
	onApply  func(SettingsValues) error
}

func DefaultSettingsValues() SettingsValues {
	return SettingsValues{
		Identify:     "shazam",
		Threshold:    0.005,
		Settle:       "2s",
		Clip:         "5s",
		MissWait:     "20s",
		FullClip:     "30s",
		Gap:          "500ms",
		Silence:      "20s",
		Recheck:      "1m",
		WeatherEvery: "15m",
		SolarEvery:   "1m",
		Celsius:      false,
		Radar:        false,
		Width:        1448,
		Height:       1072,
		Frontlight:   -1,
	}
}

// NewSettings loads path if present, otherwise starts from base.
func NewSettings(path string, base SettingsValues) (*Settings, error) {
	s := &Settings{path: path, values: base}
	if path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var file SettingsValues
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	s.values = mergeSettings(base, file)
	// Missing frontlight in older files must not become 0 (off).
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) == nil {
		if _, ok := keys["frontlight"]; !ok {
			s.values.Frontlight = base.Frontlight
		}
	}
	s.hasToken = s.values.AudDToken != ""
	return s, nil
}

func mergeSettings(base, over SettingsValues) SettingsValues {
	out := base
	if over.Identify != "" {
		out.Identify = over.Identify
	}
	if over.Threshold > 0 {
		out.Threshold = over.Threshold
	}
	if over.Settle != "" {
		out.Settle = over.Settle
	}
	if over.Clip != "" {
		out.Clip = over.Clip
	}
	if over.MissWait != "" {
		out.MissWait = over.MissWait
	}
	if over.FullClip != "" {
		out.FullClip = over.FullClip
	}
	if over.Gap != "" {
		out.Gap = over.Gap
	}
	if over.Silence != "" {
		out.Silence = over.Silence
	}
	if over.Recheck != "" {
		out.Recheck = over.Recheck
	}
	if over.WeatherEvery != "" {
		out.WeatherEvery = over.WeatherEvery
	}
	if over.SolarEvery != "" {
		out.SolarEvery = over.SolarEvery
	}
	out.Celsius = over.Celsius
	out.Radar = over.Radar
	if over.Lat != 0 || over.Lon != 0 {
		out.Lat = over.Lat
		out.Lon = over.Lon
	}
	if over.Place != "" {
		out.Place = over.Place
	}
	if over.Width > 0 {
		out.Width = over.Width
	}
	if over.Height > 0 {
		out.Height = over.Height
	}
	if over.AudDToken != "" {
		out.AudDToken = over.AudDToken
	}
	out.Frontlight = over.Frontlight
	return out
}

// Path is the JSON file on disk.
func (s *Settings) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Get returns a copy of the current values (AudD token redacted for API/HTML).
func (s *Settings) Get() SettingsValues {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.values
	if v.AudDToken != "" {
		v.AudDToken = ""
	}
	return v
}

// GetRaw returns values including the AudD token (for identify client build).
func (s *Settings) GetRaw() SettingsValues {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values
}

// HasAudDToken reports whether a token is configured.
func (s *Settings) HasAudDToken() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values.AudDToken != ""
}

// OnApply registers a hook invoked after a successful Save (hot-reload).
func (s *Settings) OnApply(fn func(SettingsValues) error) {
	s.mu.Lock()
	s.onApply = fn
	s.mu.Unlock()
}

// Save validates, persists, and applies.
func (s *Settings) Save(v SettingsValues) error {
	if err := validateSettings(v); err != nil {
		return err
	}
	s.mu.Lock()
	if v.AudDToken == "" {
		v.AudDToken = s.values.AudDToken
	}
	s.values = v
	s.hasToken = v.AudDToken != ""
	path := s.path
	apply := s.onApply
	s.mu.Unlock()

	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	if apply != nil {
		return apply(v)
	}
	return nil
}

func validateSettings(v SettingsValues) error {
	switch v.Identify {
	case "shazam", "audd", "both":
	default:
		return fmt.Errorf("identify must be shazam, audd, or both")
	}
	for _, pair := range []struct {
		name, raw string
	}{
		{"settle", v.Settle},
		{"clip", v.Clip},
		{"miss_wait", v.MissWait},
		{"full_clip", v.FullClip},
		{"gap", v.Gap},
		{"silence", v.Silence},
		{"recheck", v.Recheck},
		{"weather_every", v.WeatherEvery},
		{"solar_every", v.SolarEvery},
	} {
		if _, err := time.ParseDuration(pair.raw); err != nil {
			return fmt.Errorf("%s: %w", pair.name, err)
		}
	}
	if v.Threshold <= 0 || v.Threshold > 1 {
		return fmt.Errorf("threshold out of range")
	}
	if v.Width < 64 || v.Height < 64 {
		return fmt.Errorf("screen size too small")
	}
	if v.Frontlight < -1 || v.Frontlight > 24 {
		return fmt.Errorf("frontlight must be -1 (leave alone) or 0–24")
	}
	return nil
}

// FrontlightLevel returns the configured Kindle flIntensity, or -1 if unmanaged.
func (s *Settings) FrontlightLevel() int {
	if s == nil {
		return -1
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.values.Frontlight < -1 || s.values.Frontlight > 24 {
		return -1
	}
	return s.values.Frontlight
}

// MustDuration parses a settings duration or returns fallback.
func MustDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// FormatDuration makes dropdown-friendly strings ("1m", "500ms") instead of "1m0s".
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d%time.Hour == 0 && d >= time.Hour {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	if d%time.Minute == 0 && d >= time.Minute {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	if d%time.Second == 0 && d >= time.Second {
		return fmt.Sprintf("%ds", d/time.Second)
	}
	if d%time.Millisecond == 0 {
		return fmt.Sprintf("%dms", d/time.Millisecond)
	}
	return d.String()
}

// DefaultSettingsPath is where the Pi service keeps runtime settings.
func DefaultSettingsPath() string {
	if v := os.Getenv("KINDLE_SHAZAM_SETTINGS"); v != "" {
		return v
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/var/lib/kindle-shazam"
	}
	return filepath.Join(home, "settings.json")
}
