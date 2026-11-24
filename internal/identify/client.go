package identify

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alexgorbatchev/goshazam"
)

// Track is the part of a match the screen needs.
type Track struct {
	Key      string
	Title    string
	Artist   string
	Album    string
	Year     string
	ArtURL   string
	Offset   time.Duration
	Duration time.Duration
	Provider string // audd or shazam when racing
}

type recognizer interface {
	Identify(ctx context.Context, samples []float32) (*Track, error)
}

// Options selects the music-ID backend.
type Options struct {
	Provider  string // audd, shazam, or both (race; default)
	AudDToken string
}

// Client identifies tracks and downloads cover art.
type Client struct {
	rec   recognizer
	http  *http.Client
	name  string
	stats *Stats
}

// New builds a Client. Provider "both" (default) races AudD + Shazam.
func New(opts Options) (*Client, error) {
	httpClient := &http.Client{Timeout: 45 * time.Second}
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" || provider == "aud" || provider == "ab" || provider == "race" {
		provider = "both"
	}
	stats := newStats()
	c := &Client{http: httpClient, name: provider, stats: stats}

	shazam := &shazamRecognizer{
		shazam: goshazam.New(
			goshazam.WithLanguage("en-US"),
			goshazam.WithEndpointCountry("US"),
			goshazam.WithTimeout(12*time.Second),
		),
		http: httpClient,
	}
	token := strings.TrimSpace(opts.AudDToken)
	var audd recognizer
	if token != "" {
		audd = newAudD(token, httpClient)
	}

	switch provider {
	case "both":
		if audd == nil {
			return nil, fmt.Errorf("identify both requires AUDD_API_TOKEN (or -audd-token)")
		}
		c.rec = &raceRecognizer{shazam: shazam, audd: audd, stats: stats}
	case "audd":
		if audd == nil {
			return nil, fmt.Errorf("identify audd requires AUDD_API_TOKEN (or -audd-token)")
		}
		c.rec = audd
	case "shazam":
		c.rec = shazam
	default:
		return nil, fmt.Errorf("unknown identify provider %q (use both, audd, or shazam)", opts.Provider)
	}
	return c, nil
}

// Name is the active backend (both, audd, or shazam).
func (c *Client) Name() string { return c.name }

// Stats returns A/B counters (always non-nil).
func (c *Client) Stats() *Stats {
	if c == nil || c.stats == nil {
		return newStats()
	}
	return c.stats
}

// Identify fingerprints a mono 16 kHz clip. Nil track + nil error = no match.
func (c *Client) Identify(ctx context.Context, samples []float32) (*Track, error) {
	track, err := c.rec.Identify(ctx, samples)
	if c.name != "both" {
		matched := err == nil && track != nil
		c.stats.RecordAttempt(c.name, matched)
		if matched {
			if track.Provider == "" {
				cp := *track
				cp.Provider = c.name
				track = &cp
			}
			c.stats.RecordWin(c.name)
		}
	}
	return track, err
}

// DownloadArt fetches cover art. A failure leaves the screen with text only.
func (c *Client) DownloadArt(ctx context.Context, rawURL string) (image.Image, error) {
	if rawURL == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover art status %s", resp.Status)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return img, nil
}
