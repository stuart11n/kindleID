package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Card is one Kindle screen.
type Card struct {
	Title  string
	Artist string
	Album  string
	Year   string
	Art    image.Image
}

func albumLine(card Card) string {
	switch {
	case card.Album != "" && card.Year != "":
		return card.Album + " · " + card.Year
	case card.Year != "":
		return card.Year
	default:
		return card.Album
	}
}

// PNG encodes a grayscale screen. Width and height are the logical panel size
// before soft-rotate. Paperwhite 3 landscape is 1448×1072; portrait is
// 1072×1448. Fifth/sixth gen Paperwhites are 1024×758 landscape.
// SoftRotateCCW turns a landscape image into a portrait PNG that looks upright
// when the Kindle is laid on its side with the USB port on the left.
func PNG(width, height int, card Card) ([]byte, error) {
	img, err := drawCard(width, height, card)
	if err != nil {
		return nil, err
	}
	// Soft-rotate landscape into portrait framebuffer size so FBInk can draw
	// without changing Kindle rotation. Place the device with USB on the left.
	if width > height {
		img = SoftRotateCCW(img)
	}
	return encode(img)
}

// SoftRotateCCW rotates a landscape screen into portrait framebuffer size.
func SoftRotateCCW(src *image.Gray) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewGray(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetGray(y, w-1-x, src.GrayAt(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func drawCard(width, height int, card Card) (*image.Gray, error) {
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
	titleFace := mustFace(gobold.TTF, px(scale, 54))
	bodyFace := mustFace(goregular.TTF, px(scale, 36))
	albumFace := mustFace(goregular.TTF, px(scale, 32))

	if card.Title == "" && card.Art == nil {
		idle := mustFace(goregular.TTF, px(scale, 64))
		drawCentered(img, idle, "Nothing playing.", height/2, 0)
		return img, nil
	}

	if width > height {
		drawLandscape(img, card, pad, scale, titleFace, bodyFace, albumFace)
		return img, nil
	}
	drawPortrait(img, card, pad, scale, titleFace, bodyFace, albumFace)
	return img, nil
}

func drawPortrait(img *image.Gray, card Card, pad, scale int, titleFace, bodyFace, albumFace font.Face) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	textWidth := width - 2*pad
	titleLines := wrap(titleFace, card.Title, textWidth, 2)
	artistLines := wrap(bodyFace, card.Artist, textWidth, 2)
	albumLines := wrap(albumFace, albumLine(card), textWidth, 2)
	textH := blockHeight(titleFace, len(titleLines)) + blockHeight(bodyFace, len(artistLines)) + blockHeight(albumFace, len(albumLines))
	textH += px(scale, 18) * 2

	artSize := width - 2*pad
	room := height - pad - textH - pad
	if artSize > room {
		artSize = room
	}
	if artSize < px(scale, 200) {
		artSize = px(scale, 200)
	}

	y := pad
	if card.Art != nil {
		art := fitArt(card.Art, artSize)
		dither16(art)
		ox := (width - artSize) / 2
		draw.Draw(img, image.Rect(ox, y, ox+artSize, y+artSize), art, image.Point{}, draw.Src)
		y += artSize + px(scale, 28)
	} else {
		y = (height - textH) / 2
	}

	y = drawBlock(img, titleFace, titleLines, y, 0)
	y += px(scale, 10)
	y = drawBlock(img, bodyFace, artistLines, y, 30)
	y += px(scale, 8)
	drawBlock(img, albumFace, albumLines, y, 80)
}

func drawLandscape(img *image.Gray, card Card, pad, scale int, titleFace, bodyFace, albumFace font.Face) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	artSize := height - 2*pad
	if artSize < px(scale, 200) {
		artSize = px(scale, 200)
	}
	gap := px(scale, 36)
	textX := pad
	if card.Art != nil {
		art := fitArt(card.Art, artSize)
		dither16(art)
		oy := (height - artSize) / 2
		draw.Draw(img, image.Rect(pad, oy, pad+artSize, oy+artSize), art, image.Point{}, draw.Src)
		textX = pad + artSize + gap
	}
	textWidth := width - textX - pad
	if textWidth < px(scale, 160) {
		textWidth = px(scale, 160)
	}
	titleLines := wrap(titleFace, card.Title, textWidth, 3)
	artistLines := wrap(bodyFace, card.Artist, textWidth, 2)
	albumLines := wrap(albumFace, albumLine(card), textWidth, 2)
	textH := blockHeight(titleFace, len(titleLines)) + blockHeight(bodyFace, len(artistLines)) + blockHeight(albumFace, len(albumLines))
	textH += px(scale, 18) * 2
	y := (height - textH) / 2
	if y < pad {
		y = pad
	}
	y = drawLeft(img, titleFace, titleLines, textX, y, 0)
	y += px(scale, 10)
	y = drawLeft(img, bodyFace, artistLines, textX, y, 30)
	y += px(scale, 8)
	drawLeft(img, albumFace, albumLines, textX, y, 80)
}

var errSize = errString("screen size is too small")

type errString string

func (e errString) Error() string { return string(e) }

func px(width, at1072 int) int {
	n := int(math.Round(float64(at1072) * float64(width) / 1072))
	if n < 1 {
		return 1
	}
	return n
}

func fill(img *image.Gray, v uint8) {
	for i := range img.Pix {
		img.Pix[i] = v
	}
}

func encode(img *image.Gray) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mustFace(ttf []byte, size int) font.Face {
	parsed, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err)
	}
	return face
}

func fitArt(src image.Image, size int) *image.Gray {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	crop := image.Rect(
		b.Min.X+(b.Dx()-side)/2,
		b.Min.Y+(b.Dy()-side)/2,
		b.Min.X+(b.Dx()-side)/2+side,
		b.Min.Y+(b.Dy()-side)/2+side,
	)
	dst := image.NewGray(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Over, nil)
	return dst
}

func dither16(img *image.Gray) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	pix := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pix[y*w+x] = float64(img.GrayAt(x, y).Y)
		}
	}
	const levels = 16
	step := 255.0 / float64(levels-1)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			old := pix[i]
			if old < 0 {
				old = 0
			} else if old > 255 {
				old = 255
			}
			neu := math.Round(old/step) * step
			pix[i] = neu
			err := old - neu
			spread(pix, w, h, x+1, y, err*7/16)
			spread(pix, w, h, x-1, y+1, err*3/16)
			spread(pix, w, h, x, y+1, err*5/16)
			spread(pix, w, h, x+1, y+1, err*1/16)
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := pix[y*w+x]
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			img.SetGray(x, y, color.Gray{Y: uint8(v + 0.5)})
		}
	}
}

func spread(pix []float64, w, h, x, y int, err float64) {
	if x < 0 || y < 0 || x >= w || y >= h {
		return
	}
	pix[y*w+x] += err
}

func wrap(face font.Face, text string, maxWidth, maxLines int) []string {
	text = stringsTrim(text)
	if text == "" || maxLines < 1 {
		return nil
	}
	words := splitWords(text)
	var lines []string
	cur := ""
	for _, word := range words {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if measure(face, next) <= maxWidth || cur == "" {
			cur = next
			continue
		}
		lines = append(lines, cur)
		cur = word
		if len(lines) == maxLines-1 {
			break
		}
	}
	if cur != "" && len(lines) < maxLines {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	if len(lines) == maxLines {
		used := len(lines) - 1
		consumed := 0
		for i := 0; i < used; i++ {
			consumed += len(splitWords(lines[i]))
		}
		if consumed+len(splitWords(lines[len(lines)-1])) < len(words) {
			lines[len(lines)-1] = trimEllipsis(face, lines[len(lines)-1], maxWidth)
		}
	}
	return lines
}

func trimEllipsis(face font.Face, line string, maxWidth int) string {
	const dots = "..."
	for measure(face, line+dots) > maxWidth && len(line) > 0 {
		line = line[:len(line)-1]
	}
	return stringsTrim(line) + dots
}

func splitWords(s string) []string {
	var words []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			if cur != "" {
				words = append(words, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		words = append(words, cur)
	}
	return words
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func measure(face font.Face, s string) int {
	return font.MeasureString(face, s).Round()
}

func blockHeight(face font.Face, lines int) int {
	if lines == 0 {
		return 0
	}
	return lines * face.Metrics().Height.Round()
}

func drawBlock(img *image.Gray, face font.Face, lines []string, y int, shade uint8) int {
	height := face.Metrics().Height.Round()
	ascent := face.Metrics().Ascent.Round()
	for _, line := range lines {
		drawCentered(img, face, line, y+ascent, shade)
		y += height
	}
	return y
}

func drawLeft(img *image.Gray, face font.Face, lines []string, x, y int, shade uint8) int {
	height := face.Metrics().Height.Round()
	ascent := face.Metrics().Ascent.Round()
	for _, line := range lines {
		drawAt(img, face, line, x, y+ascent, shade)
		y += height
	}
	return y
}

func drawCentered(img *image.Gray, face font.Face, line string, baseline int, shade uint8) {
	w := measure(face, line)
	x := (img.Bounds().Dx() - w) / 2
	if x < 0 {
		x = 0
	}
	drawAt(img, face, line, x, baseline, shade)
}

func drawAt(img *image.Gray, face font.Face, line string, x, baseline int, shade uint8) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Gray{Y: shade}),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(line)
}
