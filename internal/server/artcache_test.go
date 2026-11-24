package server

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, A: 255})
		}
	}
	return img
}

func TestArtCacheFIFO(t *testing.T) {
	// Each 100x100 NRGBA ≈ 40_000 bytes. Cap at ~90k → two fit, third evicts oldest.
	c := newArtCache(90_000)
	a := solid(100, 100)
	b := solid(100, 100)
	d := solid(100, 100)

	c.put("A", "One", "http://a/1", a)
	c.put("B", "Two", "http://b/2", b)
	if c.get("A", "One", "") == nil || c.get("B", "Two", "") == nil {
		t.Fatal("expected both cached")
	}
	c.put("C", "Three", "http://c/3", d)
	if c.get("A", "One", "") != nil {
		t.Fatal("expected A evicted (FIFO)")
	}
	if c.get("B", "Two", "") == nil || c.get("C", "Three", "") == nil {
		t.Fatal("expected B and C cached")
	}
}

func TestArtCacheSkipEmptyAlbumAndURL(t *testing.T) {
	c := newArtCache(50 << 20)
	c.put("A", "", "", solid(10, 10))
	if c.get("A", "", "") != nil {
		t.Fatal("empty album+url should not cache")
	}
}

func TestArtCacheHitByURLWhenAlbumDiffers(t *testing.T) {
	c := newArtCache(50 << 20)
	img := solid(20, 20)
	c.put("Pink Floyd", "The Division Bell", "http://cdn/cover.jpg", img)
	got := c.get("Pink Floyd", "The Division Bell (Remastered)", "http://cdn/cover.jpg")
	if got == nil {
		t.Fatal("expected URL hit despite album suffix")
	}
}

func TestArtCacheHitByNormalizedAlbum(t *testing.T) {
	c := newArtCache(50 << 20)
	img := solid(20, 20)
	c.put("Pink Floyd", "The Division Bell", "", img)
	got := c.get("Pink Floyd", "The Division Bell (Remastered)", "")
	if got == nil {
		t.Fatal("expected normalized album hit")
	}
}

func TestArtCacheURLOnly(t *testing.T) {
	c := newArtCache(50 << 20)
	img := solid(20, 20)
	c.put("X", "", "http://cdn/only.jpg", img)
	if c.get("", "", "http://cdn/only.jpg") == nil {
		t.Fatal("expected URL-only hit")
	}
}
