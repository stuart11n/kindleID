package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Day is one forecast day.
type Day struct {
	Label string
	Code  int
	High  int
	Low   int
}

// Snapshot is what the idle screen draws.
type Snapshot struct {
	Place     string
	Temp      int
	Feels     int
	Humidity  int
	Wind      int
	Code      int
	Condition string
	Sunrise   time.Time
	Sunset    time.Time
	MoonFrac  float64 // 0=new … 0.5=full … 1=new
	MoonName  string
	Updated   time.Time
	Days      []Day
	Unit      string // "°F" or "°C"
	WindUnit  string
}

// Options configures Open-Meteo lookups.
type Options struct {
	Lat      float64
	Lon      float64
	Place    string
	Celsius  bool
	HTTP     *http.Client
}

// Client fetches forecasts.
type Client struct {
	opts Options
	http *http.Client
}

func New(opts Options) *Client {
	c := opts.HTTP
	if c == nil {
		// Pi 1 TLS handshakes are slow; keep this above the listen fetch budget.
		c = &http.Client{Timeout: 45 * time.Second}
	}
	return &Client{opts: opts, http: c}
}

// Resolve fills lat/lon/place from IP when coordinates were not set.
func (c *Client) Resolve(ctx context.Context) error {
	if c.opts.Lat != 0 || c.opts.Lon != 0 {
		if c.opts.Place == "" {
			c.opts.Place = fmt.Sprintf("%.2f, %.2f", c.opts.Lat, c.opts.Lon)
		}
		return nil
	}
	lat, lon, place, err := locateIP(ctx, c.http)
	if err != nil {
		return err
	}
	c.opts.Lat = lat
	c.opts.Lon = lon
	if c.opts.Place == "" {
		c.opts.Place = place
	}
	return nil
}

// Fetch returns the current conditions and a 10-day forecast.
func (c *Client) Fetch(ctx context.Context) (Snapshot, error) {
	if err := c.Resolve(ctx); err != nil {
		return Snapshot{}, err
	}
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(c.opts.Lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(c.opts.Lon, 'f', 4, 64))
	q.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,sunrise,sunset")
	q.Set("forecast_days", "10")
	q.Set("timezone", "auto")
	if c.opts.Celsius {
		q.Set("temperature_unit", "celsius")
		q.Set("wind_speed_unit", "kmh")
	} else {
		q.Set("temperature_unit", "fahrenheit")
		q.Set("wind_speed_unit", "mph")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return Snapshot{}, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Snapshot{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("open-meteo: %s", res.Status)
	}

	var raw struct {
		Current struct {
			Temp     float64 `json:"temperature_2m"`
			Humidity float64 `json:"relative_humidity_2m"`
			Feels    float64 `json:"apparent_temperature"`
			Code     int     `json:"weather_code"`
			Wind     float64 `json:"wind_speed_10m"`
			Time     string  `json:"time"`
		} `json:"current"`
		Daily struct {
			Time    []string  `json:"time"`
			Code    []int     `json:"weather_code"`
			Max     []float64 `json:"temperature_2m_max"`
			Min     []float64 `json:"temperature_2m_min"`
			Sunrise []string  `json:"sunrise"`
			Sunset  []string  `json:"sunset"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return Snapshot{}, err
	}

	unit := "°F"
	windUnit := "mph"
	if c.opts.Celsius {
		unit = "°C"
		windUnit = "km/h"
	}
	now := time.Now()
	moonFrac, moonName := MoonPhase(now)
	snap := Snapshot{
		Place:     c.opts.Place,
		Temp:      int(raw.Current.Temp + 0.5),
		Feels:     int(raw.Current.Feels + 0.5),
		Humidity:  int(raw.Current.Humidity + 0.5),
		Wind:      int(raw.Current.Wind + 0.5),
		Code:      raw.Current.Code,
		Condition: Condition(raw.Current.Code),
		MoonFrac:  moonFrac,
		MoonName:  moonName,
		Updated:   now,
		Unit:      unit,
		WindUnit:  windUnit,
	}
	if len(raw.Daily.Sunrise) > 0 {
		if t, err := time.ParseInLocation("2006-01-02T15:04", raw.Daily.Sunrise[0], time.Local); err == nil {
			snap.Sunrise = t
		}
	}
	if len(raw.Daily.Sunset) > 0 {
		if t, err := time.ParseInLocation("2006-01-02T15:04", raw.Daily.Sunset[0], time.Local); err == nil {
			snap.Sunset = t
		}
	}
	for i := range raw.Daily.Time {
		if i >= len(raw.Daily.Code) || i >= len(raw.Daily.Max) || i >= len(raw.Daily.Min) {
			break
		}
		label := dayLabel(raw.Daily.Time[i], i)
		snap.Days = append(snap.Days, Day{
			Label: label,
			Code:  raw.Daily.Code[i],
			High:  int(raw.Daily.Max[i] + 0.5),
			Low:   int(raw.Daily.Min[i] + 0.5),
		})
	}
	return snap, nil
}

func dayLabel(iso string, index int) string {
	if index == 0 {
		return "Today"
	}
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Weekday().String()[:3]
}

// Condition is a short WMO weather-code label.
func Condition(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code == 1:
		return "Mostly clear"
	case code == 2:
		return "Partly cloudy"
	case code == 3:
		return "Overcast"
	case code == 45 || code == 48:
		return "Fog"
	case code >= 51 && code <= 57:
		return "Drizzle"
	case code >= 61 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Showers"
	case code >= 85 && code <= 86:
		return "Snow showers"
	case code >= 95:
		return "Thunderstorm"
	default:
		return "Cloudy"
	}
}

func locateIP(ctx context.Context, client *http.Client) (lat, lon float64, place string, err error) {
	var errs []string
	if lat, lon, place, err = locateGeoJS(ctx, client); err == nil {
		return lat, lon, place, nil
	}
	errs = append(errs, err.Error())
	if lat, lon, place, err = locateIPAPI(ctx, client); err == nil {
		return lat, lon, place, nil
	}
	errs = append(errs, err.Error())
	if lat, lon, place, err = locateIPAPICo(ctx, client); err == nil {
		return lat, lon, place, nil
	}
	errs = append(errs, err.Error())
	return 0, 0, "", fmt.Errorf("ip locate: %s", strings.Join(errs, "; "))
}

func locateGeoJS(ctx context.Context, client *http.Client) (float64, float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://get.geojs.io/v1/ip/geo.json", nil)
	if err != nil {
		return 0, 0, "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("geojs: %s", res.Status)
	}
	var raw struct {
		Lat     string `json:"latitude"`
		Lon     string `json:"longitude"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return 0, 0, "", err
	}
	lat, err1 := strconv.ParseFloat(raw.Lat, 64)
	lon, err2 := strconv.ParseFloat(raw.Lon, 64)
	if err1 != nil || err2 != nil || (lat == 0 && lon == 0) {
		return 0, 0, "", fmt.Errorf("geojs: bad coordinates")
	}
	return lat, lon, joinPlace(raw.City, raw.Region, raw.Country), nil
}

func locateIPAPI(ctx context.Context, client *http.Client) (float64, float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://ip-api.com/json/?fields=status,message,lat,lon,city,regionName,countryCode", nil)
	if err != nil {
		return 0, 0, "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("ip-api: %s", res.Status)
	}
	var raw struct {
		Status  string  `json:"status"`
		Message string  `json:"message"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		City    string  `json:"city"`
		Region  string  `json:"regionName"`
		Country string  `json:"countryCode"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return 0, 0, "", err
	}
	if raw.Status != "success" || (raw.Lat == 0 && raw.Lon == 0) {
		if raw.Message == "" {
			raw.Message = "lookup failed"
		}
		return 0, 0, "", fmt.Errorf("ip-api: %s", raw.Message)
	}
	return raw.Lat, raw.Lon, joinPlace(raw.City, raw.Region, raw.Country), nil
}

func locateIPAPICo(ctx context.Context, client *http.Client) (float64, float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipapi.co/json/", nil)
	if err != nil {
		return 0, 0, "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("ipapi.co: %s", res.Status)
	}
	var raw struct {
		Lat     float64 `json:"latitude"`
		Lon     float64 `json:"longitude"`
		City    string  `json:"city"`
		Region  string  `json:"region"`
		Country string  `json:"country_code"`
		Error   bool    `json:"error"`
		Reason  string  `json:"reason"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return 0, 0, "", err
	}
	if raw.Error || (raw.Lat == 0 && raw.Lon == 0) {
		if raw.Reason == "" {
			raw.Reason = "unknown location"
		}
		return 0, 0, "", fmt.Errorf("ipapi.co: %s", raw.Reason)
	}
	return raw.Lat, raw.Lon, joinPlace(raw.City, raw.Region, raw.Country), nil
}

func joinPlace(city, region, country string) string {
	parts := []string{}
	if city != "" {
		parts = append(parts, city)
	}
	if region != "" && region != city {
		parts = append(parts, region)
	}
	if len(parts) == 0 && country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", ")
}
