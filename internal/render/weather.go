package render

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"kindle-shazam/internal/weather"
)

// WeatherPNG draws a TRMNL-style idle weather screen.
func WeatherPNG(width, height int, snap weather.Snapshot) ([]byte, error) {
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
		img = SoftRotateCCW(img)
	} else {
		drawWeatherPortrait(img, snap, pad, scale)
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
	drawWeatherIcon(img, snap.Code, iconX, iconY, iconSize)

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
		drawWeatherIcon(img, day.Code, cx-isz/2, forecastTop+px(scale, 40), isz)
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
	drawWeatherIcon(img, snap.Code, (width-iconSize)/2, y, iconSize)
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
		drawWeatherIcon(img, day.Code, cx-isz/2, forecastTop+px(scale, 36), isz)
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

func drawWeatherIcon(img *image.Gray, code, x, y, size int) {
	ink := uint8(20)
	cloud := uint8(210) // light grey cloud fill
	cx := x + size/2
	cy := y + size/2
	r := size / 3
	switch {
	case code == 0 || code == 1:
		fillCircle(img, cx, cy, r*2/3, ink)
		for a := 0; a < 8; a++ {
			rad := float64(a) * math.Pi / 4
			x0 := cx + int(float64(r)*0.55*math.Cos(rad))
			y0 := cy + int(float64(r)*0.55*math.Sin(rad))
			x1 := cx + int(float64(r)*1.05*math.Cos(rad))
			y1 := cy + int(float64(r)*1.05*math.Sin(rad))
			thickLine(img, x0, y0, x1, y1, ink, 3)
		}
		if code == 1 {
			drawCloud(img, cx+size/8, cy+size/6, size/3, cloud)
		}
	case code == 2:
		fillCircle(img, cx-size/6, cy-size/6, r/2, ink)
		drawCloud(img, cx, cy+size/10, size/2, cloud)
	case code == 3 || code == 45 || code == 48:
		drawCloud(img, cx, cy, size/2, cloud)
		if code >= 45 {
			for i := -2; i <= 2; i++ {
				hline(img, cx-size/3, cx+size/3, cy+size/4+i*4, 120)
			}
		}
	case code >= 51 && code <= 67 || code >= 80 && code <= 82:
		drawCloud(img, cx, cy-size/8, size/2, cloud)
		for i := -1; i <= 1; i++ {
			thickLine(img, cx+i*size/6, cy+size/8, cx+i*size/6-size/20, cy+size/3, ink, 3)
		}
	case code >= 71 && code <= 77 || code >= 85 && code <= 86:
		drawCloud(img, cx, cy-size/8, size/2, cloud)
		for i := -1; i <= 1; i++ {
			drawSnowflake(img, cx+i*size/6, cy+size/5, size/14, ink)
		}
	case code >= 95:
		drawCloud(img, cx, cy-size/8, size/2, cloud)
		thickLine(img, cx-size/12, cy+size/10, cx+size/20, cy+size/4, ink, 4)
		thickLine(img, cx+size/20, cy+size/4, cx-size/16, cy+size/3, ink, 4)
	default:
		drawCloud(img, cx, cy, size/2, cloud)
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
