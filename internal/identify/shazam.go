package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alexgorbatchev/goshazam"
	"github.com/alexgorbatchev/goshazam/pkg/audio"
	"github.com/alexgorbatchev/goshazam/pkg/models"
)

// Track is the part of a Shazam match the screen needs.
type Track struct {
	Key      string
	Title    string
	Artist   string
	Album    string
	Year     string
	ArtURL   string
	Offset   time.Duration // position in the track where the clip matched
	Duration time.Duration // track length when known
}

// Client sends a clip to Shazam.
type Client struct {
	shazam *goshazam.Shazam
	http   *http.Client
}

func New() *Client {
	return &Client{
		shazam: goshazam.New(
			goshazam.WithLanguage("en-US"),
			goshazam.WithEndpointCountry("US"),
			goshazam.WithTimeout(15*time.Second),
		),
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// Identify fingerprints about ten seconds of 16 kHz mono audio.
// A nil track and a nil error means Shazam had no match.
func (c *Client) Identify(ctx context.Context, samples []float32) (*Track, error) {
	if len(samples) < 16000 {
		return nil, fmt.Errorf("clip is only %d samples", len(samples))
	}
	pcm := make([]int16, len(samples))
	for i, v := range samples {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		pcm[i] = int16(v * 32767)
	}
	resp, err := c.shazam.Recognize(ctx, audio.NewSegment(pcm, 16000, 1))
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Track == nil || resp.Track.Title == "" || len(resp.Matches) == 0 {
		return nil, nil
	}
	track := fromResponse(resp)
	if track.Duration == 0 {
		if d, err := c.lookupDuration(ctx, resp.Track); err == nil {
			track.Duration = d
		}
	}
	return track, nil
}

func fromResponse(resp *models.ResponseTrack) *Track {
	info := resp.Track
	art := info.PhotoURL
	if art == "" && info.Images != nil {
		art = info.Images["coverarthq"]
		if art == "" {
			art = info.Images["coverart"]
		}
	}
	t := &Track{
		Key:      info.Key,
		Title:    strings.TrimSpace(info.Title),
		Artist:   strings.TrimSpace(info.Subtitle),
		Album:    metaText(info, "Album"),
		Year:     releaseYear(info),
		ArtURL:   art,
		Duration: parseDurationMeta(info),
	}
	if len(resp.Matches) > 0 && resp.Matches[0].Offset > 0 {
		t.Offset = time.Duration(resp.Matches[0].Offset * float64(time.Second))
	}
	return t
}

func metaText(info *models.TrackInfo, title string) string {
	for _, sec := range info.Sections {
		for _, meta := range sec.Metadata {
			if strings.EqualFold(meta.Title, title) {
				return strings.TrimSpace(meta.Text)
			}
		}
	}
	return ""
}

var yearRe = regexp.MustCompile(`\b(19|20)\d{2}\b`)
var durationRe = regexp.MustCompile(`(?i)(\d+)\s*:\s*(\d{2})|(\d+)\s*s(?:ec(?:onds?)?)?`)

func releaseYear(info *models.TrackInfo) string {
	for _, title := range []string{"Released", "Release", "Year"} {
		if y := yearRe.FindString(metaText(info, title)); y != "" {
			return y
		}
	}
	return ""
}

func parseDurationMeta(info *models.TrackInfo) time.Duration {
	for _, title := range []string{"Duration", "Length", "Time"} {
		if d := parseDurationText(metaText(info, title)); d > 0 {
			return d
		}
	}
	return 0
}

func parseDurationText(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	m := durationRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	if m[1] != "" && m[2] != "" {
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		return time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	}
	if m[3] != "" {
		sec, _ := strconv.Atoi(m[3])
		return time.Duration(sec) * time.Second
	}
	return 0
}

func (c *Client) lookupDuration(ctx context.Context, info *models.TrackInfo) (time.Duration, error) {
	id := appleTrackID(info)
	if id == "" {
		return 0, fmt.Errorf("no apple track id")
	}
	u := "https://itunes.apple.com/lookup?id=" + url.QueryEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("itunes lookup %s", res.Status)
	}
	var raw struct {
		Results []struct {
			TrackTimeMillis int64 `json:"trackTimeMillis"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return 0, err
	}
	if len(raw.Results) == 0 || raw.Results[0].TrackTimeMillis <= 0 {
		return 0, fmt.Errorf("itunes: no duration")
	}
	return time.Duration(raw.Results[0].TrackTimeMillis) * time.Millisecond, nil
}

var appleTrackIDRe = regexp.MustCompile(`[?&]i=(\d+)`)

func appleTrackID(info *models.TrackInfo) string {
	if info.AppleMusicURL != "" {
		if m := appleTrackIDRe.FindStringSubmatch(info.AppleMusicURL); len(m) == 2 {
			return m[1]
		}
	}
	for _, opt := range info.Hub.Options {
		for _, act := range opt.Actions {
			if act.URI == nil {
				continue
			}
			if m := appleTrackIDRe.FindStringSubmatch(*act.URI); len(m) == 2 {
				return m[1]
			}
		}
	}
	for _, act := range info.Hub.Actions {
		if act.URI == nil {
			continue
		}
		if m := appleTrackIDRe.FindStringSubmatch(*act.URI); len(m) == 2 {
			return m[1]
		}
		// applemusicplay often has type applemusicplay and id field
		if strings.EqualFold(act.Type, "applemusicplay") && act.ID != nil && *act.ID != "" {
			return *act.ID
		}
	}
	return ""
}

// DownloadArt fetches cover art. A failure leaves the screen with text only.
func (c *Client) DownloadArt(ctx context.Context, rawURL string) (image.Image, error) {
	if rawURL == "" {
		return nil, nil
	}
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
