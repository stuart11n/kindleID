package render

import (
	"testing"

	"kindle-shazam/internal/weather"
)

func TestWeatherLandscape(t *testing.T) {
	pngBytes, err := WeatherPNG(1448, 1072, weather.Snapshot{
		Place:     "Seattle, WA",
		Temp:      58,
		Feels:     55,
		Humidity:  72,
		Wind:      8,
		Code:      2,
		Condition: "Partly cloudy",
		Unit:      "°F",
		WindUnit:  "mph",
		Days: []weather.Day{
			{Label: "Today", Code: 2, High: 60, Low: 48},
			{Label: "Mon", Code: 61, High: 54, Low: 45},
			{Label: "Tue", Code: 3, High: 57, Low: 46},
			{Label: "Wed", Code: 0, High: 62, Low: 49},
			{Label: "Thu", Code: 71, High: 40, Low: 32},
			{Label: "Fri", Code: 2, High: 58, Low: 44},
			{Label: "Sat", Code: 61, High: 55, Low: 43},
			{Label: "Sun", Code: 3, High: 56, Low: 45},
			{Label: "Mon", Code: 0, High: 61, Low: 47},
			{Label: "Tue", Code: 1, High: 63, Low: 48},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, pngBytes)
	if img.Bounds().Dx() != 1072 || img.Bounds().Dy() != 1448 {
		t.Fatalf("size %v", img.Bounds())
	}
	writePreview(t, "kindle-preview-weather.png", pngBytes)
}
