package solaredge

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

// Snapshot is what the idle screen draws for solar.
type Snapshot struct {
	SiteName string

	PVWatts    float64
	LoadWatts  float64
	GridWatts  float64
	GridExport bool
	HasMeter   bool

	TodayWh    float64
	MonthWh    float64
	LifetimeWh float64

	HasBattery      bool
	BatteryPct      int
	BatteryWatts    float64
	BatteryCharging bool
	BatteryCritical bool
	Updated         time.Time
}

// Options configures the Monitoring API client.
type Options struct {
	APIKey string
	SiteID string
	HTTP   *http.Client
}

// Client talks to monitoringapi.solaredge.com (V1 site key).
type Client struct {
	opts Options
	http *http.Client
}

func New(opts Options) *Client {
	c := opts.HTTP
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{opts: opts, http: c}
}

func (c *Client) Enabled() bool {
	return strings.TrimSpace(c.opts.APIKey) != "" && strings.TrimSpace(c.opts.SiteID) != ""
}

// Fetch pulls overview + current power flow (battery %).
func (c *Client) Fetch(ctx context.Context) (Snapshot, error) {
	if !c.Enabled() {
		return Snapshot{}, fmt.Errorf("solaredge: missing api key or site id")
	}

	var overview struct {
		Overview struct {
			LastUpdateTime string `json:"lastUpdateTime"`
			LifeTimeData   struct {
				Energy float64 `json:"energy"`
			} `json:"lifeTimeData"`
			LastMonthData struct {
				Energy float64 `json:"energy"`
			} `json:"lastMonthData"`
			LastDayData struct {
				Energy float64 `json:"energy"`
			} `json:"lastDayData"`
			CurrentPower struct {
				Power float64 `json:"power"`
			} `json:"currentPower"`
		} `json:"overview"`
	}
	if err := c.get(ctx, "/site/"+url.PathEscape(c.opts.SiteID)+"/overview", &overview); err != nil {
		return Snapshot{}, err
	}

	snap := Snapshot{
		SiteName:    "Solar",
		PVWatts:     overview.Overview.CurrentPower.Power,
		TodayWh:     overview.Overview.LastDayData.Energy,
		MonthWh:     overview.Overview.LastMonthData.Energy,
		LifetimeWh: overview.Overview.LifeTimeData.Energy,
		Updated:     time.Now(),
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", overview.Overview.LastUpdateTime, time.Local); err == nil {
		snap.Updated = t
	}

	var flowResp struct {
		SiteCurrentPowerFlow struct {
			Connections []struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"connections"`
			GRID *struct {
				Status       string  `json:"status"`
				CurrentPower float64 `json:"currentPower"`
			} `json:"GRID"`
			LOAD *struct {
				Status       string  `json:"status"`
				CurrentPower float64 `json:"currentPower"`
			} `json:"LOAD"`
			PV *struct {
				Status       string  `json:"status"`
				CurrentPower float64 `json:"currentPower"`
			} `json:"PV"`
			STORAGE *struct {
				Status       string  `json:"status"`
				CurrentPower float64 `json:"currentPower"`
				ChargeLevel  int     `json:"chargeLevel"`
				Critical     bool    `json:"critical"`
			} `json:"STORAGE"`
		} `json:"siteCurrentPowerFlow"`
	}
	if err := c.get(ctx, "/site/"+url.PathEscape(c.opts.SiteID)+"/currentPowerFlow", &flowResp); err != nil {
		return snap, nil
	}
	flow := flowResp.SiteCurrentPowerFlow
	if flow.PV != nil {
		snap.PVWatts = flow.PV.CurrentPower
	}
	if flow.LOAD != nil {
		snap.LoadWatts = flow.LOAD.CurrentPower
	}
	if flow.GRID != nil {
		snap.GridWatts = flow.GRID.CurrentPower
	}
	charging := false
	discharging := false
	for _, conn := range flow.Connections {
		from := strings.ToUpper(conn.From)
		to := strings.ToUpper(conn.To)
		if from == "LOAD" && to == "GRID" {
			snap.GridExport = true
		}
		if from == "GRID" && to == "LOAD" {
			snap.GridExport = false
		}
		if to == "STORAGE" {
			charging = true
		}
		if from == "STORAGE" {
			discharging = true
		}
	}
	if flow.STORAGE != nil {
		snap.HasBattery = true
		snap.BatteryPct = flow.STORAGE.ChargeLevel
		snap.BatteryWatts = flow.STORAGE.CurrentPower
		snap.BatteryCritical = flow.STORAGE.Critical
		snap.BatteryCharging = charging && !discharging
		if discharging && !charging {
			snap.BatteryCharging = false
		}
	}
	return snap, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	q := url.Values{}
	q.Set("api_key", c.opts.APIKey)
	u := "https://monitoringapi.solaredge.com" + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("solaredge %s: %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(dest)
}

// FormatPower renders watts as W or kW.
func FormatPower(watts float64) string {
	a := watts
	if a < 0 {
		a = -a
	}
	if a >= 1000 {
		return strconv.FormatFloat(watts/1000, 'f', 1, 64) + " kW"
	}
	return strconv.FormatFloat(watts, 'f', 0, 64) + " W"
}

// FormatEnergy renders watt-hours as kWh/MWh.
func FormatEnergy(wh float64) string {
	if wh >= 1_000_000 {
		return strconv.FormatFloat(wh/1_000_000, 'f', 1, 64) + " MWh"
	}
	if wh >= 1000 {
		return strconv.FormatFloat(wh/1000, 'f', 1, 64) + " kWh"
	}
	return strconv.FormatFloat(wh, 'f', 0, 64) + " Wh"
}
