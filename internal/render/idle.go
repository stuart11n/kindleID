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

	"kindle-shazam/internal/solaredge"
	"kindle-shazam/internal/weather"
)

// Idle is the nothing-playing dashboard.
type Idle struct {
	Weather       *weather.Snapshot
	Solar         *solaredge.Snapshot
	Satellite     image.Image
	ShowSatellite bool
}

func (idle Idle) hasRightPanel() bool {
	if idle.ShowSatellite && idle.Satellite != nil {
		return true
	}
	return idle.Solar != nil
}

// IdlePNG draws weather, and SolarEdge stats when present.
func IdlePNG(width, height int, idle Idle) ([]byte, error) {
	if idle.Weather != nil && !idle.hasRightPanel() {
		return WeatherPNG(width, height, *idle.Weather)
	}
	if idle.Weather == nil && !idle.hasRightPanel() {
		return PNG(width, height, Card{})
	}
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
		drawIdleLandscape(img, idle, pad, scale)
		img = SoftRotateCCW(img)
	} else {
		drawIdlePortrait(img, idle, pad, scale)
	}
	return encode(img)
}

func drawIdleLandscape(img *image.Gray, idle Idle, pad, scale int) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	mid := width / 2
	vline(img, mid, pad, height-pad-px(scale, 250), 200)

	if idle.Weather != nil {
		drawWeatherColumn(img, *idle.Weather, pad, mid-pad, pad, height-pad-px(scale, 250), scale)
	}
	drawIdleRight(img, idle, mid+pad, width-pad, pad, height-pad-px(scale, 250), scale)

	if idle.Weather == nil || len(idle.Weather.Days) == 0 {
		return
	}
	dividerY := height - pad - px(scale, 250)
	hline(img, pad, width-pad, dividerY, 180)
	days := idle.Weather.Days
	if len(days) > 10 {
		days = days[:10]
	}
	dayFace := mustFace(goregular.TTF, px(scale, 22))
	hiFace := mustFace(gobold.TTF, px(scale, 28))
	loFace := mustFace(goregular.TTF, px(scale, 24))
	colW := (width - 2*pad) / len(days)
	forecastTop := dividerY + px(scale, 24)
	for i, day := range days {
		x := pad + i*colW
		cx := x + colW/2
		drawCenteredIn(img, dayFace, day.Label, x, colW, forecastTop+dayFace.Metrics().Ascent.Round(), 40)
		isz := px(scale, 60)
		drawWeatherIcon(img, day.Code, cx-isz/2, forecastTop+px(scale, 34), isz)
		hy := forecastTop + px(scale, 128)
		drawCenteredIn(img, hiFace, fmt.Sprintf("%d°", day.High), x, colW, hy+hiFace.Metrics().Ascent.Round(), 0)
		drawCenteredIn(img, loFace, fmt.Sprintf("%d°", day.Low), x, colW, hy+hiFace.Metrics().Height.Round()+px(scale, 6)+loFace.Metrics().Ascent.Round(), 90)
	}
}

func drawIdlePortrait(img *image.Gray, idle Idle, pad, scale int) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	split := height * 45 / 100
	if idle.Weather != nil {
		drawWeatherColumn(img, *idle.Weather, pad, width-pad, pad, split-pad, scale)
	}
	hline(img, pad, width-pad, split, 180)
	drawIdleRight(img, idle, pad, width-pad, split+pad, height-pad, scale)
}

func drawIdleRight(img *image.Gray, idle Idle, x0, x1, y0, y1, scale int) {
	if idle.ShowSatellite && idle.Satellite != nil {
		drawSatelliteColumn(img, idle.Satellite, x0, x1, y0, y1, scale)
		return
	}
	if idle.Solar != nil {
		drawSolarColumn(img, *idle.Solar, x0, x1, y0, y1, scale)
	}
}

func drawWeatherColumn(img *image.Gray, snap weather.Snapshot, x0, x1, y0, y1, scale int) {
	placeFace := mustFace(goregular.TTF, px(scale, 26))
	tempFace := mustFace(gobold.TTF, px(scale, 148))
	condFace := mustFace(goregular.TTF, px(scale, 40))
	metaFace := mustFace(goregular.TTF, px(scale, 24))
	sunFace := mustFace(goregular.TTF, px(scale, 28))
	moonFace := mustFace(goregular.TTF, px(scale, 40))

	colW := x1 - x0
	y := y0
	if snap.Place != "" {
		drawAt(img, placeFace, snap.Place, x0, y+placeFace.Metrics().Ascent.Round(), 80)
		y += placeFace.Metrics().Height.Round() + px(scale, 6)
	}
	icon := px(scale, 220)
	if icon > colW*45/100 {
		icon = colW * 45 / 100
	}
	drawWeatherIcon(img, snap.Code, x0, y, icon)
	tx := x0 + icon + px(scale, 20)
	tempStr := fmt.Sprintf("%d%s", snap.Temp, snap.Unit)
	tempW := measure(tempFace, tempStr)
	tempCX := tx + tempW/2
	drawAt(img, tempFace, tempStr, tx, y+tempFace.Metrics().Ascent.Round(), 0)
	ty := y + tempFace.Metrics().Height.Round()
	condW := measure(condFace, snap.Condition)
	drawAt(img, condFace, snap.Condition, tempCX-condW/2, ty+condFace.Metrics().Ascent.Round(), 20)
	y += icon + px(scale, 16)
	if y < ty+condFace.Metrics().Height.Round()+px(scale, 12) {
		y = ty + condFace.Metrics().Height.Round() + px(scale, 12)
	}
	meta := fmt.Sprintf("Feels %d%s  ·  Humidity %d%%  ·  Wind %d %s", snap.Feels, snap.Unit, snap.Humidity, snap.Wind, snap.WindUnit)
	metaW := measure(metaFace, meta)
	drawAt(img, metaFace, meta, x0+(colW-metaW)/2, y+metaFace.Metrics().Ascent.Round(), 60)
	y += metaFace.Metrics().Height.Round() + px(scale, 18)

	if !snap.Sunrise.IsZero() || !snap.Sunset.IsZero() {
		y += px(scale, 12)
		drawSunTimes(img, snap.Sunrise, snap.Sunset, x0, x1, y, scale, sunFace)
		y += px(scale, 64)
	}
	if snap.MoonName != "" {
		y += px(scale, 16)
		// Weather glyphs use ~size/3 radius; size the moon to 75% of that visual.
		weatherGlyph := icon * 2 / 3
		moon := weatherGlyph * 75 / 100
		nameW := measure(moonFace, snap.MoonName)
		gap := moon/2 + 40
		nameX := tempCX - nameW/2
		moonX := nameX - gap - moon + 5
		drawMoonRow(img, snap.MoonFrac, snap.MoonName, moonX, y, moon, moonFace)
	}
	_ = y1
}

func drawMoonRow(img *image.Gray, frac float64, name string, x, y, icon int, face font.Face) {
	drawMoonIcon(img, x, y, icon, frac)
	gap := icon/2 + 40
	drawAt(img, face, name, x+icon+gap, y+icon/2+face.Metrics().Ascent.Round()/3, 20)
}

// drawMoonIcon paints a northern-hemisphere moon. frac 0=new, 0.5=full.
func drawMoonIcon(img *image.Gray, x, y, size int, frac float64) {
	ink := uint8(30)
	lite := uint8(210)
	cx := x + size/2
	cy := y + size/2
	r := size / 2
	if r < 6 {
		r = 6
	}
	// disk outline + dark fill
	fillCircle(img, cx, cy, r, lite)
	rectOutlineCircle(img, cx, cy, r, ink)

	frac = math.Mod(frac, 1)
	if frac < 0 {
		frac += 1
	}
	if frac < 0.02 || frac > 0.98 {
		fillCircle(img, cx, cy, r-1, ink)
		return
	}
	if frac > 0.48 && frac < 0.52 {
		return // full moon: light disk already
	}

	// Illuminated portion: waxing = right lit, waning = left lit.
	waxing := frac < 0.5
	phase := frac
	if !waxing {
		phase = frac - 0.5
	}
	// phase 0..0.5 within half-cycle; terminator offset from -r..+r
	var term float64
	if waxing {
		if phase <= 0.25 {
			term = -float64(r) + 4*float64(r)*phase // new→quarter: -r → 0
		} else {
			term = 4*float64(r)*(phase-0.25) // quarter→full: 0 → +r
		}
	} else {
		if phase <= 0.25 {
			term = float64(r) - 4*float64(r)*phase // full→quarter: +r → 0
		} else {
			term = -4 * float64(r) * (phase - 0.25) // quarter→new: 0 → -r
		}
	}

	for dy := -r; dy <= r; dy++ {
		rowHalf := int(math.Sqrt(float64(r*r - dy*dy)) + 0.5)
		for dx := -rowHalf; dx <= rowHalf; dx++ {
			lit := false
			if waxing {
				// dark on left of terminator ellipse edge
				edge := int(term * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(r*r))))
				lit = dx >= edge
			} else {
				edge := int(term * math.Sqrt(math.Max(0, 1-float64(dy*dy)/float64(r*r))))
				lit = dx <= edge
			}
			if !lit {
				setInk(img, cx+dx, cy+dy, ink)
			}
		}
	}
}

func rectOutlineCircle(img *image.Gray, cx, cy, r int, shade uint8) {
	for a := 0; a < 360; a += 3 {
		rad := float64(a) * math.Pi / 180
		setInk(img, cx+int(float64(r)*math.Cos(rad)+0.5), cy+int(float64(r)*math.Sin(rad)+0.5), shade)
	}
}

func drawSunTimes(img *image.Gray, sunrise, sunset time.Time, x0, x1, y, scale int, face font.Face) {
	colW := x1 - x0
	icon := px(scale, 56)
	gap := px(scale, 12)
	pairGap := px(scale, 36)
	baseline := y + icon/2 + face.Metrics().Ascent.Round()/3

	type item struct {
		label string
		rise  bool
	}
	var items []item
	if !sunrise.IsZero() {
		items = append(items, item{label: formatClock(sunrise), rise: true})
	}
	if !sunset.IsZero() {
		items = append(items, item{label: formatClock(sunset), rise: false})
	}
	if len(items) == 0 {
		return
	}

	total := 0
	widths := make([]int, len(items))
	for i, it := range items {
		widths[i] = icon + gap + measure(face, it.label)
		total += widths[i]
	}
	total += pairGap * (len(items) - 1)
	x := x0 + (colW-total)/2
	for i, it := range items {
		if it.rise {
			drawSunriseIcon(img, x, y, icon)
		} else {
			drawSunsetIcon(img, x, y, icon)
		}
		drawAt(img, face, it.label, x+icon+gap, baseline, 20)
		x += widths[i] + pairGap
	}
}

func formatClock(t time.Time) string {
	h := t.Hour() % 12
	if h == 0 {
		h = 12
	}
	ampm := "AM"
	if t.Hour() >= 12 {
		ampm = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", h, t.Minute(), ampm)
}

// Sunrise / sunset icons follow the classic half-sun-on-horizon mark:
// filled semicircle for sunrise, outline for sunset, seven upper rays,
// horizon line, and two short reflection strokes below.
func drawSunriseIcon(img *image.Gray, x, y, size int) {
	drawHorizonSun(img, x, y, size, true)
}

func drawSunsetIcon(img *image.Gray, x, y, size int) {
	drawHorizonSun(img, x, y, size, false)
}

func drawHorizonSun(img *image.Gray, x, y, size int, filled bool) {
	ink := uint8(25)
	cx := x + size/2
	horizon := y + size*5/8
	r := size * 28 / 100
	if r < 8 {
		r = 8
	}
	thick := size / 18
	if thick < 2 {
		thick = 2
	}

	// Seven rays above the horizon (center longest). −150°..−30° keeps
	// the outer rays clear of the horizon line.
	rayLens := []float64{1.55, 1.9, 2.25, 2.55, 2.25, 1.9, 1.55}
	for i, mul := range rayLens {
		ang := -5*math.Pi/6 + float64(i)*math.Pi/9
		inner := float64(r) * 1.2
		outer := float64(r) * mul
		x0 := cx + int(inner*math.Cos(ang))
		y0 := horizon + int(inner*math.Sin(ang))
		x1 := cx + int(outer*math.Cos(ang))
		y1 := horizon + int(outer*math.Sin(ang))
		thickLine(img, x0, y0, x1, y1, ink, thick)
	}

	// Upper semicircle sitting on the horizon.
	if filled {
		fillUpperSemi(img, cx, horizon, r, ink)
	} else {
		strokeUpperSemi(img, cx, horizon, r, ink, thick)
	}

	// Horizon and reflection lines.
	hw := r * 2
	hlineThick(img, cx-hw, cx+hw, horizon, ink, thick)
	hlineThick(img, cx-hw*3/4, cx+hw*3/4, horizon+thick*2+2, ink, thick)
	hlineThick(img, cx-hw/2, cx+hw/2, horizon+thick*4+4, ink, thick)
}

func fillUpperSemi(img *image.Gray, cx, cy, r int, shade uint8) {
	r2 := r * r
	for dy := -r; dy <= 0; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r2 {
				setInk(img, cx+dx, cy+dy, shade)
			}
		}
	}
}

func strokeUpperSemi(img *image.Gray, cx, cy, r int, shade uint8, thick int) {
	for a := 180; a <= 360; a++ {
		rad := float64(a) * math.Pi / 180
		px := cx + int(float64(r)*math.Cos(rad)+0.5)
		py := cy + int(float64(r)*math.Sin(rad)+0.5)
		for t := -thick / 2; t <= thick/2; t++ {
			setInk(img, px+t, py, shade)
			setInk(img, px, py+t, shade)
		}
	}
}

func hlineThick(img *image.Gray, x0, x1, y int, shade uint8, thick int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for t := -thick / 2; t <= thick/2; t++ {
		hline(img, x0, x1, y+t, shade)
	}
}

func rectOutline(img *image.Gray, x, y, w, h int, shade uint8) {
	hline(img, x, x+w, y, shade)
	hline(img, x, x+w, y+h-1, shade)
	vline(img, x, y, y+h, shade)
	vline(img, x+w-1, y, y+h, shade)
}

func vline(img *image.Gray, x, y0, y1 int, shade uint8) {
	if x < 0 || x >= img.Bounds().Dx() {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y < y1; y++ {
		if y >= 0 && y < img.Bounds().Dy() {
			img.SetGray(x, y, color.Gray{Y: shade})
		}
	}
}
