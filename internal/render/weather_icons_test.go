package render

import "testing"

func TestWeatherIconNameMapping(t *testing.T) {
	cases := []struct {
		code  int
		night bool
		want  string
	}{
		{0, false, "sunny"},
		{2, false, "sunny_intervals"},
		{2, true, "partly_cloudy_night"},
		{3, false, "grey_cloud"},
		{45, false, "fog"},
		{51, false, "drizzle"},
		{61, false, "light_rain"},
		{65, false, "heavy_rain"},
		{80, false, "light_rain_shower"},
		{80, true, "light_rain_shower_night"},
		{71, false, "light_snow"},
		{75, false, "heavy_snow"},
		{95, false, "thunder_storm"},
	}
	ensureWeatherIcons()
	for _, c := range cases {
		got := weatherIconName(c.code, c.night)
		if got != c.want {
			t.Fatalf("code %d night=%v: got %q want %q", c.code, c.night, got, c.want)
		}
		if weatherIconMap[got] == nil {
			t.Fatalf("missing embedded icon %q", got)
		}
	}
}
