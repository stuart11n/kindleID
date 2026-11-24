package identify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type auddRecognizer struct {
	token string
	http  *http.Client
}

func newAudD(token string, httpClient *http.Client) *auddRecognizer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	return &auddRecognizer{token: strings.TrimSpace(token), http: httpClient}
}

func (a *auddRecognizer) Identify(ctx context.Context, samples []float32) (*Track, error) {
	if a.token == "" {
		return nil, fmt.Errorf("audd: missing api token")
	}
	if len(samples) < 16000 {
		return nil, fmt.Errorf("clip is only %d samples", len(samples))
	}
	// Standard AudD endpoint analyzes ~first 12s of the uploaded file;
	// send the most recent 12s so we fingerprint what's playing now.
	clip := lastSeconds(samples, 16000, 12)
	wav := encodeWAV(clip, 16000)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("api_token", a.token)
	_ = w.WriteField("return", "apple_music,spotify")
	part, err := w.CreateFormFile("file", "clip.wav")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(wav); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.audd.io/", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audd http %s: %s", res.Status, truncate(string(raw), 200))
	}

	var parsed struct {
		Status string `json:"status"`
		Error  struct {
			ErrorCode    int    `json:"error_code"`
			ErrorMessage string `json:"error_message"`
		} `json:"error"`
		Result *struct {
			Artist      string `json:"artist"`
			Title       string `json:"title"`
			Album       string `json:"album"`
			ReleaseDate string `json:"release_date"`
			Timecode    string `json:"timecode"`
			SongLink    string `json:"song_link"`
			AppleMusic  *struct {
				URL     string `json:"url"`
				Artwork *struct {
					URL string `json:"url"`
				} `json:"artwork"`
			} `json:"apple_music"`
			Spotify *struct {
				ID     string `json:"id"`
				Album  *struct {
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"spotify"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("audd decode: %w", err)
	}
	if parsed.Status != "success" {
		msg := parsed.Error.ErrorMessage
		if msg == "" {
			msg = truncate(string(raw), 200)
		}
		return nil, fmt.Errorf("audd: %s", msg)
	}
	if parsed.Result == nil || strings.TrimSpace(parsed.Result.Title) == "" {
		return nil, nil
	}
	r := parsed.Result
	t := &Track{
		Key:    "audd:" + strings.ToLower(strings.TrimSpace(r.Artist)) + "|" + strings.ToLower(strings.TrimSpace(r.Title)),
		Title:  strings.TrimSpace(r.Title),
		Artist: strings.TrimSpace(r.Artist),
		Album:  strings.TrimSpace(r.Album),
		Year:   yearFromDate(r.ReleaseDate),
		Offset: parseTimecode(r.Timecode),
	}
	if r.Spotify != nil && r.Spotify.ID != "" {
		t.Key = "spotify:" + r.Spotify.ID
	}
	if r.AppleMusic != nil && r.AppleMusic.Artwork != nil {
		t.ArtURL = expandAppleArt(r.AppleMusic.Artwork.URL)
	}
	if t.ArtURL == "" && r.Spotify != nil && r.Spotify.Album != nil && len(r.Spotify.Album.Images) > 0 {
		t.ArtURL = r.Spotify.Album.Images[0].URL
	}
	return t, nil
}

func yearFromDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 4 && s[0] >= '1' && s[0] <= '2' {
		return s[:4]
	}
	return ""
}

func parseTimecode(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0
	}
	min, err1 := parseInt(parts[0])
	sec, err2 := parseInt(parts[1])
	if err1 != nil || err2 != nil {
		return 0
	}
	return time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad int")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func expandAppleArt(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	u = strings.ReplaceAll(u, "{w}", "600")
	u = strings.ReplaceAll(u, "{h}", "600")
	return u
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
