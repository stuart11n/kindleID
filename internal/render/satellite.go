package render

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
	"golang.org/x/image/font/gofont/goregular"
)

// drawSatelliteColumn fits a SoCal radar still into the right panel.
func drawSatelliteColumn(img *image.Gray, sat image.Image, x0, x1, y0, y1, scale int) {
	if sat == nil {
		return
	}
	colW := x1 - x0
	colH := y1 - y0
	if colW < 8 || colH < 8 {
		return
	}

	titleFace := mustFace(goregular.TTF, px(scale, 24))
	title := "SoCal radar"
	drawAt(img, titleFace, title, x0, y0+titleFace.Metrics().Ascent.Round(), 80)
	top := y0 + titleFace.Metrics().Height.Round() + px(scale, 10)

	dstW := colW
	dstH := y1 - top
	if dstW < 8 || dstH < 8 {
		return
	}

	sb := sat.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw < 1 || sh < 1 {
		return
	}

	// Cover the panel (center-crop), then convert to grayscale for e-ink.
	scaleX := float64(dstW) / float64(sw)
	scaleY := float64(dstH) / float64(sh)
	s := scaleX
	if scaleY > s {
		s = scaleY
	}
	tw := int(float64(sw)*s + 0.5)
	th := int(float64(sh)*s + 0.5)
	scaled := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), sat, sb, draw.Over, nil)

	ox := (tw - dstW) / 2
	oy := (th - dstH) / 2
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			r, g, b, _ := scaled.At(ox+dx, oy+dy).RGBA()
			// Rec. 601 luma → 8-bit gray.
			y := (r*299 + g*587 + b*114 + 500) / 1000 >> 8
			img.SetGray(x0+dx, top+dy, color.Gray{Y: uint8(y)})
		}
	}
}
