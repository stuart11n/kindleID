package identify

import (
	"context"
	"encoding/json"
	"fmt"
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

type shazamRecognizer struct {
	shazam *goshazam.Shazam
	http   *http.Client
}

// Identify fingerprints a mono 16 kHz clip via unofficial goshazam.
// A nil track and a nil error means no match.
// Keeps the fingerprint short — signature generation is CPU-heavy on Pi 1.
func (c *shazamRecognizer) Identify(ctx context.Context, samples []float32) (*Track, error) {
	if len(samples) < 16000 {
		return nil, fmt.Errorf("clip is only %d samples", len(samples))
	}
	// Cap at 8s: longer windows crush ARMv6 fingerprinting and rarely help.
	samples = lastSeconds(samples, 16000, 8)
	peak, _ := peakStats(samples)
	return c.recognize(ctx, prepareClip(samples, peak, true))
}

func (c *shazamRecognizer) recognize(ctx context.Context, samples []float32) (*Track, error) {
	pcm := make([]int16, len(samples))
	for i, v := range samples {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		pcm[i] = int16(v * 32767)
	}
	seg := audio.NewSegment(pcm, 16000, 1)
	sg := audio.CreateSignatureGenerator(seg)
	sg.SamplesProcessed = 0
	maxSec := seg.DurationSeconds()
	if maxSec > 8 {
		maxSec = 8
	}
	if maxSec < 1 {
		maxSec = 1
	}
	sg.MaxTimeSeconds = maxSec
	sig := sg.GetNextSignature()
	if sig == nil {
		return nil, nil
	}
	resp, err := c.shazam.RecognizeSignature(ctx, sig)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Track == nil || resp.Track.Title == "" || len(resp.Matches) == 0 {
		return nil, nil
	}
	// Skip iTunes duration lookup — extra RTT; AudD/recheck fallback is enough.
	return fromResponse(resp), nil
}

func peakStats(samples []float32) (peak float32, clipPct float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	var clipped int
	for _, v := range samples {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
		if v >= 0.99 {
			clipped++
		}
	}
	return peak, float64(clipped) / float64(len(samples))
}

func prepareClip(samples []float32, peak float32, boostQuiet bool) []float32 {
	if len(samples) == 0 || peak < 1e-4 {
		return samples
	}
	var g float32
	switch {
	case peak >= 0.95:
		g = 0.7 / peak
	case boostQuiet && peak < 0.85:
		g = 0.85 / peak
	case !boostQuiet:
		g = 0.5
	default:
		return samples
	}
	out := make([]float32, len(samples))
	for i, v := range samples {
		x := v * g
		if x > 1 {
			x = 1
		} else if x < -1 {
			x = -1
		}
		out[i] = x
	}
	return out
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

func (c *shazamRecognizer) lookupDuration(ctx context.Context, info *models.TrackInfo) (time.Duration, error) {
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
		if strings.EqualFold(act.Type, "applemusicplay") && act.ID != nil && *act.ID != "" {
			return *act.ID
		}
	}
	return ""
}
