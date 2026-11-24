package server

import (
	"image"
	"strings"
	"sync"
)

const defaultArtCacheBytes = 50 << 20 // 50 MiB

type artEntry struct {
	img  image.Image
	size int
	keys []string // all keys that point at this entry
}

// artCache is a FIFO cover-art store capped by approximate decoded size.
// Entries are keyed by artist|album and/or art URL so AudD/Shazam naming
// mismatches still hit when the cover URL is the same.
type artCache struct {
	mu    sync.Mutex
	max   int
	used  int
	order []string // FIFO of primary keys (first key inserted for each entry)
	items map[string]*artEntry
}

func newArtCache(maxBytes int) *artCache {
	if maxBytes <= 0 {
		maxBytes = defaultArtCacheBytes
	}
	return &artCache{
		max:   maxBytes,
		items: make(map[string]*artEntry),
	}
}

func albumCacheKey(artist, album string) string {
	a := normalizeAlbum(album)
	if a == "" {
		return ""
	}
	return "a:" + strings.ToLower(strings.TrimSpace(artist)) + "|" + a
}

func urlCacheKey(artURL string) string {
	u := strings.TrimSpace(artURL)
	if u == "" {
		return ""
	}
	return "u:" + u
}

func normalizeAlbum(album string) string {
	s := strings.ToLower(strings.TrimSpace(album))
	if s == "" {
		return ""
	}
	// Drop common edition noise so remasters still share a cover.
	for _, cut := range []string{
		" (remastered)", " - remastered", " remastered",
		" (deluxe)", " - deluxe",
		" (expanded edition)", " (anniversary edition)",
	} {
		if i := strings.Index(s, cut); i > 0 {
			s = strings.TrimSpace(s[:i])
		}
	}
	return s
}

func coverKeys(artist, album, artURL string) []string {
	var keys []string
	if k := albumCacheKey(artist, album); k != "" {
		keys = append(keys, k)
	}
	if k := urlCacheKey(artURL); k != "" {
		keys = append(keys, k)
	}
	return keys
}

func (c *artCache) get(artist, album, artURL string) image.Image {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range coverKeys(artist, album, artURL) {
		if e := c.items[k]; e != nil {
			return e.img
		}
	}
	return nil
}

func (c *artCache) put(artist, album, artURL string, img image.Image) {
	keys := coverKeys(artist, album, artURL)
	if len(keys) == 0 || img == nil || c == nil {
		return
	}
	size := imageBytes(img)
	if size <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// If any key already exists, refresh that entry and alias the rest.
	var existing *artEntry
	for _, k := range keys {
		if e := c.items[k]; e != nil {
			existing = e
			break
		}
	}
	if existing != nil {
		c.used -= existing.size
		existing.img = img
		existing.size = size
		c.used += size
		for _, k := range keys {
			if c.items[k] != existing {
				c.items[k] = existing
				existing.keys = appendUnique(existing.keys, k)
			}
		}
		c.evictLocked()
		return
	}

	e := &artEntry{img: img, size: size, keys: append([]string(nil), keys...)}
	for _, k := range keys {
		c.items[k] = e
	}
	c.order = append(c.order, keys[0])
	c.used += size
	c.evictLocked()
}

func appendUnique(keys []string, k string) []string {
	for _, x := range keys {
		if x == k {
			return keys
		}
	}
	return append(keys, k)
}

func (c *artCache) evictLocked() {
	for c.used > c.max && len(c.order) > 0 {
		old := c.order[0]
		c.order = c.order[1:]
		e := c.items[old]
		if e == nil {
			continue
		}
		c.used -= e.size
		for _, k := range e.keys {
			if c.items[k] == e {
				delete(c.items, k)
			}
		}
	}
	if c.used < 0 {
		c.used = 0
	}
}

func (c *artCache) len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.order)
}

func imageBytes(img image.Image) int {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0
	}
	switch img.(type) {
	case *image.Gray, *image.Alpha, *image.Gray16:
		return w * h
	case *image.NRGBA, *image.RGBA, *image.NRGBA64, *image.RGBA64, *image.NYCbCrA:
		return w * h * 4
	case *image.YCbCr:
		return w * h * 2 // approx 4:2:0
	default:
		return w * h * 4
	}
}
