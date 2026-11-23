package render

import (
	"testing"
	"time"

	"kindle-shazam/internal/solaredge"
	"kindle-shazam/internal/weather"
)

func TestIdleWeatherAndSolar(t *testing.T) {
	wx := weather.Snapshot{
		Place:     "El Cajon",
		Temp:      66,
		Feels:     64,
		Humidity:  40,
		Wind:      5,
		Code:      1,
		Condition: "Mostly clear",
		Unit:      "°F",
		WindUnit:  "mph",
		Sunrise:   time.Date(2026, 11, 22, 6, 24, 0, 0, time.Local),
		Sunset:    time.Date(2026, 11, 22, 16, 43, 0, 0, time.Local),
		MoonFrac:  0.12,
		MoonName:  "Waxing crescent",
		Days: []weather.Day{
			{Label: "Today", Code: 1, High: 68, Low: 48},
			{Label: "Mon", Code: 2, High: 70, Low: 50},
			{Label: "Tue", Code: 3, High: 65, Low: 49},
			{Label: "Wed", Code: 0, High: 67, Low: 48},
			{Label: "Thu", Code: 61, High: 62, Low: 47},
			{Label: "Fri", Code: 2, High: 66, Low: 49},
			{Label: "Sat", Code: 1, High: 69, Low: 50},
			{Label: "Sun", Code: 3, High: 64, Low: 48},
			{Label: "Mon", Code: 0, High: 68, Low: 49},
			{Label: "Tue", Code: 2, High: 70, Low: 51},
		},
	}
	solar := solaredge.Snapshot{
		SiteName:        "Solar",
		PVWatts:         4200,
		LoadWatts:       1100,
		GridWatts:       2800,
		GridExport:      true,
		HasMeter:        true,
		TodayWh:         18400,
		MonthWh:         420000,
		HasBattery:      true,
		BatteryPct:      73,
		BatteryCharging: true,
	}
	pngBytes, err := IdlePNG(1448, 1072, Idle{Weather: &wx, Solar: &solar})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, pngBytes)
	if img.Bounds().Dx() != 1072 || img.Bounds().Dy() != 1448 {
		t.Fatalf("size %v", img.Bounds())
	}
	writePreview(t, "kindle-preview-idle-solar.png", pngBytes)
}
