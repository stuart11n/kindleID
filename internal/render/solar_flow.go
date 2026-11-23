package render

import (
	"fmt"
	"image"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"kindle-shazam/internal/solaredge"
)

func drawSolarColumn(img *image.Gray, snap solaredge.Snapshot, x0, x1, y0, y1, scale int) {
	titleFace := mustFace(goregular.TTF, px(scale, 26))
	valFace := mustFace(gobold.TTF, px(scale, 48))
	leafFace := mustFace(gobold.TTF, px(scale, 32))
	metaFace := mustFace(goregular.TTF, px(scale, 22))

	y := y0
	name := snap.SiteName
	if name == "" {
		name = "Solar"
	}
	drawAt(img, titleFace, name, x0, y+titleFace.Metrics().Ascent.Round(), 80)
	y += titleFace.Metrics().Height.Round() + px(scale, 4)

	meta := ""
	if snap.TodayWh > 0 {
		meta = "Today  " + solaredge.FormatEnergy(snap.TodayWh)
	}
	if snap.MonthWh > 0 {
		if meta != "" {
			meta += "   ·   "
		}
		meta += "Month  " + solaredge.FormatEnergy(snap.MonthWh)
	}
	if meta != "" {
		drawAt(img, metaFace, meta, x0, y+metaFace.Metrics().Ascent.Round(), 70)
		y += metaFace.Metrics().Height.Round() + px(scale, 12)
	} else {
		y += px(scale, 8)
	}

	drawSolarFlowDiagram(img, snap, x0, x1, y, y1, scale, valFace, leafFace)
}

// drawSolarFlowDiagram sketches Solar → Inverter → {Home, Battery, Grid}.
func drawSolarFlowDiagram(img *image.Gray, snap solaredge.Snapshot, x0, x1, y0, y1, scale int, valFace, leafFace font.Face) {
	ink := uint8(30)
	margin := px(scale, 56)
	y0 += margin
	y1 -= margin
	if y1 <= y0+px(scale, 160) {
		y1 = y0 + px(scale, 160)
	}
	colW := x1 - x0
	availH := y1 - y0
	if availH < px(scale, 200) {
		availH = px(scale, 200)
	}

	cx := x0 + colW/2
	icon := px(scale, 64)
	if icon > colW/5 {
		icon = colW / 5
	}
	gapY := availH / 14
	if gapY < px(scale, 10) {
		gapY = px(scale, 10)
	}

	valH := valFace.Metrics().Height.Round() + px(scale, 8)
	leafValH := leafFace.Metrics().Height.Round() + px(scale, 8)

	nLeaves := 2
	if snap.HasBattery {
		nLeaves = 3
	}
	leafSlot := colW / nLeaves
	homeX := x0 + leafSlot/2
	gridX := x0 + colW - leafSlot/2
	battX := cx
	if !snap.HasBattery {
		homeX = x0 + colW/4
		gridX = x0 + 3*colW/4
	}

	// Reserve bottom for leaf icons + values, then place inverter hub above the branch gap.
	leafBlock := icon + leafValH + px(scale, 8)
	leafY := y1 - leafBlock
	branchGap := px(scale, 56)
	if branchGap > availH/6 {
		branchGap = availH / 6
	}
	solarIcon := icon * 3 / 2
	invY := leafY - branchGap - icon
	solarBlock := solarIcon + valH
	solarY := y0
	if invY-gapY-4 < solarY+solarBlock {
		// Compress when the column is short.
		solarY = y0
		invY = solarY + solarIcon + gapY + px(scale, 20)
		leafY = invY + icon + branchGap
	}

	// Solar — panel + live value, then arrow into the inverter.
	drawSolarPanelIcon(img, cx-solarIcon/2, solarY, solarIcon, ink)
	sy := solarY + solarIcon + px(scale, 4)
	drawCenteredIn(img, valFace, solaredge.FormatPower(snap.PVWatts), x0, colW, sy+valFace.Metrics().Ascent.Round(), 0)
	sy += valFace.Metrics().Height.Round() + px(scale, 6)
	drawFlowArrow(img, cx, sy, cx, invY-2, true, ink)

	// Inverter hub — icon only (no label).
	drawInverterIcon(img, cx-icon/2, invY, icon, ink)
	invBottom := invY + icon
	invCX := cx

	// Home
	drawHomeIcon(img, homeX-icon/2, leafY, icon, ink)
	ly := leafY + icon + px(scale, 4)
	homeVal := "—"
	if snap.HasMeter || snap.LoadWatts > 0 {
		homeVal = solaredge.FormatPower(snap.LoadWatts)
	}
	drawCenteredIn(img, leafFace, homeVal, homeX-leafSlot/2, leafSlot, ly+leafFace.Metrics().Ascent.Round(), 0)
	drawFlowElbow(img, invCX, invBottom, homeX, leafY, true, ink)

	// Grid
	drawGridIcon(img, gridX-icon/2, leafY, icon, ink)
	gridVal := "—"
	fromHubToGrid := true
	gridDir := 0 // -1 down (import), +1 up (export)
	if snap.HasMeter {
		if snap.GridWatts < 25 {
			gridVal = "idle"
		} else if snap.GridExport {
			gridVal = solaredge.FormatPower(snap.GridWatts)
			fromHubToGrid = true
			gridDir = -1 // ↓ export to grid
		} else {
			gridVal = solaredge.FormatPower(snap.GridWatts)
			fromHubToGrid = false
			gridDir = 1 // ↑ import from grid
		}
	}
	gridBaseline := ly + leafFace.Metrics().Ascent.Round()
	if gridDir != 0 {
		drawCenteredPowerDir(img, leafFace, gridVal, gridDir > 0, gridX-leafSlot/2, leafSlot, gridBaseline, 0)
	} else {
		drawCenteredIn(img, leafFace, gridVal, gridX-leafSlot/2, leafSlot, gridBaseline, 0)
	}
	drawFlowElbow(img, invCX, invBottom, gridX, leafY, fromHubToGrid, ink)

	// Battery
	if snap.HasBattery {
		drawBatteryIcon(img, battX-icon/2, leafY, icon, snap.BatteryPct, ink)
		battPct := fmt.Sprintf("%d%%", snap.BatteryPct)
		if snap.BatteryCritical {
			battPct += " low"
		}
		battBaseline := ly + leafFace.Metrics().Ascent.Round()
		toBatt := snap.BatteryCharging || snap.BatteryWatts < 25
		battFlow := !snap.BatteryCritical && (snap.BatteryWatts > 25 || snap.BatteryCharging)
		if battFlow {
			power := ""
			if snap.BatteryWatts > 25 {
				power = solaredge.FormatPower(snap.BatteryWatts)
			}
			// Match grid: into storage ↓, out of storage ↑
			drawCenteredPctPowerDir(img, leafFace, battPct, power, !toBatt, battX-leafSlot/2, leafSlot, battBaseline, 0)
		} else {
			drawCenteredIn(img, leafFace, battPct, battX-leafSlot/2, leafSlot, battBaseline, 0)
		}
		drawFlowArrow(img, invCX, invBottom+2, battX, leafY-2, toBatt, ink)
	}
}

func drawSolarPanelIcon(img *image.Gray, x, y, size int, ink uint8) {
	pad := size * 4 / 100
	if pad < 1 {
		pad = 1
	}
	panelY := y + pad
	panelH := size - 2*pad
	panelW := size - 2*pad
	left := x + pad
	fillRect(img, left, panelY, panelW, panelH, 210) // light grey cells
	rectOutline(img, left, panelY, panelW, panelH, ink)
	vline(img, left+panelW/3, panelY, panelY+panelH, ink)
	vline(img, left+2*panelW/3, panelY, panelY+panelH, ink)
	hline(img, left, left+panelW, panelY+panelH/2, ink)
	hline(img, left, left+panelW, panelY+panelH/4, ink)
	hline(img, left, left+panelW, panelY+3*panelH/4, ink)
}

func fillRect(img *image.Gray, x, y, w, h int, shade uint8) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			setInk(img, x+dx, y+dy, shade)
		}
	}
}

func drawInverterIcon(img *image.Gray, x, y, size int, ink uint8) {
	pad := size / 10
	rectOutline(img, x+pad, y+pad, size-2*pad, size-2*pad, ink)
	cx := x + size/2
	cy := y + size/2
	thickLine(img, cx-size/5, cy, cx-size/12, cy-size/6, ink, 3)
	thickLine(img, cx-size/12, cy-size/6, cx+size/12, cy+size/6, ink, 3)
	thickLine(img, cx+size/12, cy+size/6, cx+size/5, cy, ink, 3)
}

func drawHomeIcon(img *image.Gray, x, y, size int, ink uint8) {
	cx := x + size/2
	roofY := y + size/8
	eaveY := y + size*42/100
	leftEave, rightEave := x+size/8, x+size-size/8
	fillTriangle(img, cx, roofY, leftEave, eaveY, rightEave, eaveY, 210)
	bodyX := x + size*22/100
	bodyW := size * 56 / 100
	bodyH := size * 48 / 100
	thickLine(img, cx, roofY, leftEave, eaveY, ink, 1)
	thickLine(img, cx, roofY, rightEave, eaveY, ink, 1)
	thickLine(img, leftEave, eaveY, rightEave, eaveY, ink, 1)
	rectOutline(img, bodyX, eaveY, bodyW, bodyH, ink)
	doorW := bodyW / 4
	doorH := bodyH * 55 / 100
	doorX := cx - doorW/2
	doorY := eaveY + bodyH - doorH
	rectOutline(img, doorX, doorY, doorW, doorH, ink)
}

func fillTriangle(img *image.Gray, x0, y0, x1, y1, x2, y2 int, shade uint8) {
	minY, maxY := y0, y0
	if y1 < minY {
		minY = y1
	}
	if y2 < minY {
		minY = y2
	}
	if y1 > maxY {
		maxY = y1
	}
	if y2 > maxY {
		maxY = y2
	}
	for y := minY; y <= maxY; y++ {
		xs := make([]int, 0, 3)
		xs = appendEdgeX(xs, x0, y0, x1, y1, y)
		xs = appendEdgeX(xs, x1, y1, x2, y2, y)
		xs = appendEdgeX(xs, x2, y2, x0, y0, y)
		if len(xs) < 2 {
			continue
		}
		lo, hi := xs[0], xs[0]
		for _, v := range xs[1:] {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		for x := lo; x <= hi; x++ {
			setInk(img, x, y, shade)
		}
	}
}

func appendEdgeX(xs []int, x0, y0, x1, y1, y int) []int {
	if y0 == y1 {
		if y == y0 {
			xs = append(xs, x0, x1)
		}
		return xs
	}
	if (y < y0 && y < y1) || (y > y0 && y > y1) {
		return xs
	}
	t := float64(y-y0) / float64(y1-y0)
	xs = append(xs, x0+int(t*float64(x1-x0)+0.5))
	return xs
}

func drawGridIcon(img *image.Gray, x, y, size int, ink uint8) {
	cx := x + size/2
	top := y + size/10
	bot := y + size - size/10
	thickLine(img, cx, top, cx, bot, ink, 3)
	for i, yy := range []int{top + size/6, top + size*38/100, top + size*58/100} {
		arm := size * (28 - i*4) / 100
		thickLine(img, cx-arm, yy, cx+arm, yy, ink, 2)
		thickLine(img, cx-arm, yy, cx, yy+size/10, ink, 2)
		thickLine(img, cx+arm, yy, cx, yy+size/10, ink, 2)
	}
	thickLine(img, cx, bot-size/8, cx-size/4, bot, ink, 2)
	thickLine(img, cx, bot-size/8, cx+size/4, bot, ink, 2)
}

func drawBatteryIcon(img *image.Gray, x, y, size, pct int, ink uint8) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	bodyW := size * 55 / 100
	bodyH := size * 72 / 100
	bx := x + (size-bodyW)/2
	by := y + size*18/100
	rectOutline(img, bx, by, bodyW, bodyH, ink)
	nubW := bodyW / 3
	nubH := size / 10
	rectOutline(img, bx+(bodyW-nubW)/2, by-nubH, nubW, nubH, ink)
	inner := 3
	fillH := (bodyH - 2*inner) * pct / 100
	for dy := 0; dy < fillH; dy++ {
		for dx := 0; dx < bodyW-2*inner; dx++ {
			setInk(img, bx+inner+dx, by+bodyH-inner-1-dy, ink)
		}
	}
}

// drawCenteredPowerDir draws power with a leading ↑ or ↓.
func drawCenteredPowerDir(img *image.Gray, face font.Face, power string, up bool, x, width, baseline int, shade uint8) {
	drawCenteredPctPowerDir(img, face, "", power, up, x, width, baseline, shade)
}

// drawCenteredPctPowerDir draws optional pct, an ↑/↓, and optional power, centered as a group.
func drawCenteredPctPowerDir(img *image.Gray, face font.Face, pct, power string, up bool, x, width, baseline int, shade uint8) {
	ah := face.Metrics().Height.Round() * 55 / 100
	if ah < 14 {
		ah = 14
	}
	gap := ah / 3
	pctW := 0
	if pct != "" {
		pctW = measure(face, pct)
	}
	pw := 0
	if power != "" {
		pw = measure(face, power)
	}
	total := ah
	if pctW > 0 {
		total += pctW + gap
	}
	if pw > 0 {
		total += gap + pw
	}
	left := x + (width-total)/2
	if pctW > 0 {
		drawAt(img, face, pct, left, baseline, shade)
		left += pctW + gap
	}
	ax := left + ah/2
	ay := baseline - face.Metrics().Ascent.Round()/2
	drawUpDownArrow(img, ax, ay, ah, up, shade)
	left += ah
	if pw > 0 {
		drawAt(img, face, power, left+gap, baseline, shade)
	}
}

func drawUpDownArrow(img *image.Gray, cx, cy, size int, up bool, ink uint8) {
	half := size / 2
	thick := size / 7
	if thick < 2 {
		thick = 2
	}
	top, bot := cy-half, cy+half
	thickLine(img, cx, top, cx, bot, ink, thick)
	if up {
		thickLine(img, cx, top, cx-half*2/3, top+half*2/3, ink, thick)
		thickLine(img, cx, top, cx+half*2/3, top+half*2/3, ink, thick)
	} else {
		thickLine(img, cx, bot, cx-half*2/3, bot-half*2/3, ink, thick)
		thickLine(img, cx, bot, cx+half*2/3, bot-half*2/3, ink, thick)
	}
}

func drawFlowArrow(img *image.Gray, x0, y0, x1, y1 int, forward bool, ink uint8) {
	if !forward {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	thickLine(img, x0, y0, x1, y1, ink, 2)
	drawArrowHead(img, x0, y0, x1, y1, ink)
}

func drawFlowElbow(img *image.Gray, hx, hy, lx, ly int, fromHub bool, ink uint8) {
	midY := (hy + ly) / 2
	thickLine(img, hx, hy, hx, midY, ink, 2)
	thickLine(img, hx, midY, lx, midY, ink, 2)
	thickLine(img, lx, midY, lx, ly, ink, 2)
	if fromHub {
		drawArrowHead(img, lx, midY, lx, ly, ink)
	} else {
		drawArrowHead(img, hx, midY, hx, hy, ink)
	}
}

func drawArrowHead(img *image.Gray, x0, y0, x1, y1 int, ink uint8) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	length := math.Hypot(dx, dy)
	if length < 1 {
		return
	}
	ux, uy := dx/length, dy/length
	const size = 10.0
	px, py := -uy, ux
	ax := float64(x1) - ux*size + px*size*0.55
	ay := float64(y1) - uy*size + py*size*0.55
	bx := float64(x1) - ux*size - px*size*0.55
	by := float64(y1) - uy*size - py*size*0.55
	thickLine(img, x1, y1, int(ax+0.5), int(ay+0.5), ink, 2)
	thickLine(img, x1, y1, int(bx+0.5), int(by+0.5), ink, 2)
}
