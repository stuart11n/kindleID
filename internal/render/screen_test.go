package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestNothingPlayingLandscape(t *testing.T) {
	pngBytes, err := PNG(1448, 1072, Card{})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, pngBytes)
	// Soft-rotated into portrait framebuffer size.
	if img.Bounds().Dx() != 1072 || img.Bounds().Dy() != 1448 {
		t.Fatalf("size %v", img.Bounds())
	}
	if img.GrayAt(10, 10).Y != 255 {
		t.Fatalf("background %d", img.GrayAt(10, 10).Y)
	}
	writePreview(t, "kindle-preview-idle.png", pngBytes)
}

func TestTrackCardLandscape(t *testing.T) {
	art := image.NewRGBA(image.Rect(0, 0, 400, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 400; x++ {
			art.SetRGBA(x, y, color.RGBA{R: uint8(x * 255 / 400), G: uint8(y * 255 / 500), B: 80, A: 255})
		}
	}
	pngBytes, err := PNG(1448, 1072, Card{
		Title:  "The Chain",
		Artist: "Fleetwood Mac",
		Album:  "Rumours",
		Year:   "1977",
		Art:    art,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, pngBytes)
	if img.Bounds().Dx() != 1072 || img.Bounds().Dy() != 1448 {
		t.Fatalf("size %v", img.Bounds())
	}
	// After CCW soft-rotate, left-side art lands near the bottom of portrait.
	if img.GrayAt(536, 1200).Y == 255 {
		t.Fatal("album art was not drawn")
	}
	writePreview(t, "kindle-preview-track.png", pngBytes)
}

func TestPortraitStillWorks(t *testing.T) {
	pngBytes, err := PNG(1072, 1448, Card{
		Title:  "The Chain",
		Artist: "Fleetwood Mac",
		Album:  "Rumours",
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, pngBytes)
	if img.Bounds().Dx() != 1072 || img.Bounds().Dy() != 1448 {
		t.Fatalf("size %v", img.Bounds())
	}
}

func TestOlderPaperwhiteSize(t *testing.T) {
	pngBytes, err := PNG(758, 1024, Card{Title: "Hello", Artist: "Adele", Album: "25"})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, pngBytes)
	if img.Bounds().Dx() != 758 || img.Bounds().Dy() != 1024 {
		t.Fatalf("size %v", img.Bounds())
	}
}

func decode(t *testing.T, raw []byte) *image.Gray {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	g, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("png is %T, want gray", img)
	}
	return g
}

func writePreview(t *testing.T, name string, raw []byte) {
	t.Helper()
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}
