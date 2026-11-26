package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"kindle-shazam/internal/weather"
)

// WeatherPNG draws a TRMNL-style idle weather screen.
func WeatherPNG(width, height int, snap weather.Snapshot) ([]byte, error) {
	return weatherPNG(width, height, snap, "")
}

func weatherPNG(width, height int, snap weather.Snapshot, footer string) ([]byte, error) {
	if width < 64 || height < 64 {
		return nil, errSize
	}
	img := image.NewGray(image.Rect(0, 0, width, height))
	fill(img, 255)
	scale := width
	if height < scale {
		scale = height
	}
	pad := scale / 18
	if pad < 16 {
		pad = 16
	}
	if width > height {
		drawWeatherLandscape(img, snap, pad, scale)
		drawFooter(img, scale, footer)
		img = SoftRotateCCW(img)
	} else {
		drawWeatherPortrait(img, snap, pad, scale)
		drawFooter(img, scale, footer)
	}
	return encode(img)
}

func drawWeatherLandscape(img *image.Gray, snap weather.Snapshot, pad, scale int) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	placeFace := mustFace(goregular.TTF, px(scale, 28))
	tempFace := mustFace(gobold.TTF, px(scale, 160))
	condFace := mustFace(goregular.TTF, px(scale, 40))
	metaFace := mustFace(goregular.TTF, px(scale, 30))
	dayFace := mustFace(goregular.TTF, px(scale, 22))
	hiFace := mustFace(gobold.TTF, px(scale, 28))
	loFace := mustFace(goregular.TTF, px(scale, 24))

	iconSize := px(scale, 220)
	iconX := pad
	iconY := pad + px(scale, 20)
	drawWeatherIcon(img, snap.Code, iconX, iconY, iconSize, isNightAt(snap.Sunrise, snap.Sunset, time.Now()), snap.MoonFrac)

	textX := iconX + iconSize + px(scale, 40)
	y := pad
	if snap.Place != "" {
		drawAt(img, placeFace, snap.Place, textX, y+placeFace.Metrics().Ascent.Round(), 80)
		y += placeFace.Metrics().Height.Round() + px(scale, 8)
	}
	temp := fmt.Sprintf("%d%s", snap.Temp, snap.Unit)
	drawAt(img, tempFace, temp, textX, y+tempFace.Metrics().Ascent.Round(), 0)
	y += tempFace.Metrics().Height.Round() - px(scale, 10)
	drawAt(img, condFace, snap.Condition, textX, y+condFace.Metrics().Ascent.Round(), 20)
	y += condFace.Metrics().Height.Round() + px(scale, 18)

	meta := fmt.Sprintf("Feels %d%s   Humidity %d%%   Wind %d %s", snap.Feels, snap.Unit, snap.Humidity, snap.Wind, snap.WindUnit)
	drawAt(img, metaFace, meta, textX, y+metaFace.Metrics().Ascent.Round(), 60)

	dividerY := height - pad - px(scale, 260)
	hline(img, pad, width-pad, dividerY, 180)

	days := snap.Days
	if len(days) > 10 {
		days = days[:10]
	}
	if len(days) == 0 {
		return
	}
	colW := (width - 2*pad) / len(days)
	forecastTop := dividerY + px(scale, 28)
	for i, day := range days {
		cx := pad + i*colW + colW/2
		x := pad + i*colW
		drawCenteredIn(img, dayFace, day.Label, x, colW, forecastTop+dayFace.Metrics().Ascent.Round(), 40)
		isz := px(scale, 52)
		drawWeatherIcon(img, day.Code, cx-isz/2, forecastTop+px(scale, 40), isz, false, 0)
		hi := fmt.Sprintf("%d°", day.High)
		lo := fmt.Sprintf("%d°", day.Low)
		hy := forecastTop + px(scale, 130)
		drawCenteredIn(img, hiFace, hi, x, colW, hy+hiFace.Metrics().Ascent.Round(), 0)
		drawCenteredIn(img, loFace, lo, x, colW, hy+hiFace.Metrics().Height.Round()+px(scale, 8)+loFace.Metrics().Ascent.Round(), 90)
	}
}

func drawWeatherPortrait(img *image.Gray, snap weather.Snapshot, pad, scale int) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	placeFace := mustFace(goregular.TTF, px(scale, 28))
	tempFace := mustFace(gobold.TTF, px(scale, 140))
	condFace := mustFace(goregular.TTF, px(scale, 36))
	metaFace := mustFace(goregular.TTF, px(scale, 28))
	dayFace := mustFace(goregular.TTF, px(scale, 20))
	hiFace := mustFace(gobold.TTF, px(scale, 26))
	loFace := mustFace(goregular.TTF, px(scale, 22))

	y := pad
	if snap.Place != "" {
		drawCentered(img, placeFace, snap.Place, y+placeFace.Metrics().Ascent.Round(), 80)
		y += placeFace.Metrics().Height.Round() + px(scale, 12)
	}
	iconSize := px(scale, 180)
	drawWeatherIcon(img, snap.Code, (width-iconSize)/2, y, iconSize, isNightAt(snap.Sunrise, snap.Sunset, time.Now()), snap.MoonFrac)
	y += iconSize + px(scale, 16)
	temp := fmt.Sprintf("%d%s", snap.Temp, snap.Unit)
	drawCentered(img, tempFace, temp, y+tempFace.Metrics().Ascent.Round(), 0)
	y += tempFace.Metrics().Height.Round()
	drawCentered(img, condFace, snap.Condition, y+condFace.Metrics().Ascent.Round(), 20)
	y += condFace.Metrics().Height.Round() + px(scale, 12)
	meta := fmt.Sprintf("Feels %d%s · Humidity %d%% · Wind %d %s", snap.Feels, snap.Unit, snap.Humidity, snap.Wind, snap.WindUnit)
	drawCentered(img, metaFace, meta, y+metaFace.Metrics().Ascent.Round(), 60)

	dividerY := height - pad - px(scale, 280)
	hline(img, pad, width-pad, dividerY, 180)
	days := snap.Days
	if len(days) > 10 {
		days = days[:10]
	}
	if len(days) == 0 {
		return
	}
	colW := (width - 2*pad) / len(days)
	forecastTop := dividerY + px(scale, 24)
	for i, day := range days {
		x := pad + i*colW
		cx := x + colW/2
		drawCenteredIn(img, dayFace, day.Label, x, colW, forecastTop+dayFace.Metrics().Ascent.Round(), 40)
		isz := px(scale, 48)
		drawWeatherIcon(img, day.Code, cx-isz/2, forecastTop+px(scale, 36), isz, false, 0)
		hy := forecastTop + px(scale, 120)
		drawCenteredIn(img, hiFace, fmt.Sprintf("%d°", day.High), x, colW, hy+hiFace.Metrics().Ascent.Round(), 0)
		drawCenteredIn(img, loFace, fmt.Sprintf("%d°", day.Low), x, colW, hy+hiFace.Metrics().Height.Round()+px(scale, 6)+loFace.Metrics().Ascent.Round(), 90)
	}
}

func drawCenteredIn(img *image.Gray, face font.Face, line string, x, width, baseline int, shade uint8) {
	w := measure(face, line)
	drawAt(img, face, line, x+(width-w)/2, baseline, shade)
}

func hline(img *image.Gray, x0, x1, y int, shade uint8) {
	if y < 0 || y >= img.Bounds().Dy() {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for x := x0; x < x1; x++ {
		if x >= 0 && x < img.Bounds().Dx() {
			img.SetGray(x, y, color.Gray{Y: shade})
		}
	}
}

func drawWeatherIcon(img *image.Gray, code, x, y, size int, night bool, moonFrac float64) {
	// Clear night: smaller moon-phase glyph (same drawing as the phase row).
	if code == 0 && night {
		moon := size * 38 / 100
		if moon < 10 {
			moon = 10
		}
		drawMoonIcon(img, x+(size-moon)/2, y+(size-moon)/2, moon, moonFrac)
		return
	}
	ensureWeatherIcons()
	if src := weatherIconMap[weatherIconName(code, night)]; src != nil {
		blitGrayScaled(img, src, x, y, size)
		return
	}
	// Fallback procedural glyph if an asset is missing.
	ink := uint8(20)
	cloud := uint8(210)
	cx := x + size/2
	cy := y + size/2
	r := size / 3
	switch {
	case code == 0 || code == 1:
		sunR := r * 2 / 3
		strokeCircle(img, cx, cy, sunR, ink, 3)
		drawSunRays(img, cx, cy, sunR, ink, 3)
	case code == 2:
		sx, sy, sr := cx-size/6, cy-size/6, r/2
		strokeCircle(img, sx, sy, sr, ink, 3)
		drawSunRays(img, sx, sy, sr, ink, 3)
		drawCloud(img, cx, cy+size/10, size/2, cloud)
	default:
		drawCloud(img, cx, cy, size/2, cloud)
	}
}

// drawSunRays paints eight short strokes outside the sun disc.
func drawSunRays(img *image.Gray, cx, cy, sunR int, shade uint8, thick int) {
	inner := float64(sunR) * 1.25
	outer := float64(sunR) * 1.85
	for a := 0; a < 8; a++ {
		rad := float64(a) * math.Pi / 4
		x0 := cx + int(inner*math.Cos(rad))
		y0 := cy + int(inner*math.Sin(rad))
		x1 := cx + int(outer*math.Cos(rad))
		y1 := cy + int(outer*math.Sin(rad))
		thickLine(img, x0, y0, x1, y1, shade, thick)
	}
}

func drawCloud(img *image.Gray, cx, cy, size int, shade uint8) {
	fillCircle(img, cx-size/3, cy, size/3, shade)
	fillCircle(img, cx+size/4, cy, size/3, shade)
	fillCircle(img, cx, cy-size/4, size*2/5, shade)
	fillCircle(img, cx, cy+size/10, size/3, shade)
}

func drawSnowflake(img *image.Gray, cx, cy, r int, shade uint8) {
	thickLine(img, cx-r, cy, cx+r, cy, shade, 2)
	thickLine(img, cx, cy-r, cx, cy+r, shade, 2)
	thickLine(img, cx-r*2/3, cy-r*2/3, cx+r*2/3, cy+r*2/3, shade, 2)
	thickLine(img, cx-r*2/3, cy+r*2/3, cx+r*2/3, cy-r*2/3, shade, 2)
}

func fillCircle(img *image.Gray, cx, cy, r int, shade uint8) {
	if r < 1 {
		return
	}
	r2 := r * r
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r2 {
				setInk(img, cx+dx, cy+dy, shade)
			}
		}
	}
}

func strokeCircle(img *image.Gray, cx, cy, r int, shade uint8, thick int) {
	if r < 1 {
		return
	}
	if thick < 1 {
		thick = 1
	}
	// Midpoint-ish ring: paint pixels near radius r.
	outer2 := (r + thick/2) * (r + thick/2)
	inner := r - (thick+1)/2
	inner2 := 0
	if inner > 0 {
		inner2 = inner * inner
	}
	for dy := -r - thick; dy <= r+thick; dy++ {
		for dx := -r - thick; dx <= r+thick; dx++ {
			d2 := dx*dx + dy*dy
			if d2 <= outer2 && d2 >= inner2 {
				setInk(img, cx+dx, cy+dy, shade)
			}
		}
	}
}

func thickLine(img *image.Gray, x0, y0, x1, y1 int, shade uint8, thickness int) {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		for t := -thickness / 2; t <= thickness/2; t++ {
			setInk(img, x0+t, y0, shade)
			setInk(img, x0, y0+t, shade)
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func setInk(img *image.Gray, x, y int, shade uint8) {
	b := img.Bounds()
	if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
		return
	}
	img.SetGray(x, y, color.Gray{Y: shade})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
