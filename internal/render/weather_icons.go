package render

import (
	"bytes"
	"embed"
	"image"
	"image/draw"
	"image/png"
	"strings"
	"sync"
	"time"
)

//go:embed weathericons/*.png
var weatherIconFS embed.FS

var (
	weatherIconsOnce sync.Once
	weatherIconMap   map[string]*image.Gray
)

func ensureWeatherIcons() {
	weatherIconsOnce.Do(func() {
		weatherIconMap = map[string]*image.Gray{}
		entries, err := weatherIconFS.ReadDir("weathericons")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
				continue
			}
			raw, err := weatherIconFS.ReadFile("weathericons/" + e.Name())
			if err != nil {
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			g := toGray(img)
			name := strings.TrimSuffix(e.Name(), ".png")
			weatherIconMap[name] = g
		}
	})
}

func toGray(src image.Image) *image.Gray {
	b := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// weatherIconName maps a WMO weather code (+ day/night) to a sheet label key.
func weatherIconName(code int, night bool) string {
	switch {
	case code == 0:
		// Clear night is drawn as the moon-phase glyph, not a sheet icon.
		return "sunny"
	case code == 1:
		if night {
			return "partly_cloudy_night"
		}
		return "sunny_intervals"
	case code == 2:
		if night {
			return "partly_cloudy_night"
		}
		return "sunny_intervals"
	case code == 3:
		return "grey_cloud"
	case code == 45 || code == 48:
		return "fog"
	case code >= 51 && code <= 57:
		return "drizzle"
	case code == 61 || code == 63:
		return "light_rain"
	case code == 65:
		return "heavy_rain"
	case code == 66 || code == 67:
		return "sleet"
	case code >= 71 && code <= 77:
		if code >= 75 {
			return "heavy_snow"
		}
		return "light_snow"
	case code == 80:
		if night {
			return "light_rain_shower_night"
		}
		return "light_rain_shower"
	case code == 81 || code == 82:
		if night {
			return "heavy_rain_shower_night"
		}
		return "heavy_rain_shower"
	case code == 85:
		if night {
			return "light_snow_shower_night"
		}
		return "light_snow_shower"
	case code == 86:
		if night {
			return "heavy_snow_shower_night"
		}
		return "heavy_snow_shower"
	case code == 95:
		return "thunder_storm"
	case code == 96 || code == 99:
		return "hail"
	case code == 97 || code == 98:
		if night {
			return "thundery_shower_night"
		}
		return "thundery_shower"
	default:
		return "white_cloud"
	}
}

func isNightAt(sunrise, sunset, now time.Time) bool {
	if sunrise.IsZero() || sunset.IsZero() || now.IsZero() {
		return false
	}
	// Open-Meteo sunrise/sunset are wall times for the site day.
	if now.Before(sunrise) || !now.Before(sunset) {
		return true
	}
	return false
}

func blitGrayScaled(dst *image.Gray, src *image.Gray, x, y, size int) {
	if src == nil || size < 1 {
		return
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw < 1 || sh < 1 {
		return
	}
	db := dst.Bounds()
	for dy := 0; dy < size; dy++ {
		sy := sb.Min.Y + dy*sh/size
		for dx := 0; dx < size; dx++ {
			sx := sb.Min.X + dx*sw/size
			px := x + dx
			py := y + dy
			if px < db.Min.X || py < db.Min.Y || px >= db.Max.X || py >= db.Max.Y {
				continue
			}
			v := src.GrayAt(sx, sy).Y
			if v >= 250 {
				continue // keep destination (transparent white)
			}
			dst.SetGray(px, py, src.GrayAt(sx, sy))
		}
	}
}
