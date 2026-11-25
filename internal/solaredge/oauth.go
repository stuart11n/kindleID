package solaredge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL          = "https://monitoringapi.solaredge.com/v2/oauth2/token"
	apiV2Base         = "https://monitoringapi.solaredge.com/v2"
	tokenExpiryMargin = 5 * time.Minute
)

// OAuthOptions configures SolarEdge Monitoring API V2 (Site Access OAuth).
type OAuthOptions struct {
	ClientID     string
	ClientSecret string
	SiteID       string
	RefreshToken string // optional seed; rotated tokens are persisted to TokenPath
	TokenPath    string // JSON file for access + refresh tokens
	HTTP         *http.Client
}

type tokenFile struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	SiteID       string    `json:"site_id,omitempty"`
}

// OAuthClient talks to monitoringapi.solaredge.com/v2 with a rotating refresh token.
type OAuthClient struct {
	opts OAuthOptions
	http *http.Client

	mu    sync.Mutex
	token tokenFile
}

func NewOAuth(opts OAuthOptions) *OAuthClient {
	c := opts.HTTP
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	if opts.TokenPath == "" {
		opts.TokenPath = DefaultTokenPath()
	}
	oc := &OAuthClient{opts: opts, http: c}
	if t, err := loadTokenFile(opts.TokenPath); err == nil {
		oc.token = t
	}
	if strings.TrimSpace(opts.RefreshToken) != "" && oc.token.RefreshToken == "" {
		oc.token.RefreshToken = strings.TrimSpace(opts.RefreshToken)
	}
	if oc.token.SiteID == "" {
		oc.token.SiteID = strings.TrimSpace(opts.SiteID)
	}
	return oc
}

// DefaultTokenPath is where rotated OAuth tokens are stored on the Pi.
func DefaultTokenPath() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, "solaredge-token.json")
	}
	return "/var/lib/kindle-shazam/solaredge-token.json"
}

func (c *OAuthClient) Enabled() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.opts.ClientID) != "" &&
		strings.TrimSpace(c.opts.ClientSecret) != "" &&
		strings.TrimSpace(c.siteID()) != "" &&
		strings.TrimSpace(c.refreshToken()) != ""
}

func (c *OAuthClient) siteID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token.SiteID != "" {
		return c.token.SiteID
	}
	return c.opts.SiteID
}

func (c *OAuthClient) refreshToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token.RefreshToken != "" {
		return c.token.RefreshToken
	}
	return c.opts.RefreshToken
}

// Fetch builds a power-flow-like snapshot from Basic-tier endpoints.
// (Live /power-flow is Advanced-only and returns 403 on Free/Site Access.)
func (c *OAuthClient) Fetch(ctx context.Context) (Snapshot, error) {
	if !c.Enabled() {
		return Snapshot{}, fmt.Errorf("solaredge oauth: missing client id/secret, site id, or refresh token")
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	site := c.siteID()

	snap := Snapshot{SiteName: "Solar", Updated: time.Now()}

	pv, pvErr := c.getCurrentPower(ctx, token, site)
	if pvErr != nil {
		return Snapshot{}, pvErr
	}
	snap.PVWatts = pv

	var chargeW, dischargeW float64
	_ = c.applyStorageTelemetry(ctx, token, site, &snap, &chargeW, &dischargeW)

	var importW, exportW float64
	if err := c.applyMeterTelemetry(ctx, token, site, &importW, &exportW); err == nil {
		snap.HasMeter = true
		if exportW >= importW && exportW > 1 {
			snap.GridWatts = exportW
			snap.GridExport = true
		} else {
			snap.GridWatts = importW
			snap.GridExport = false
		}
	}

	// Do not invent PV from battery charge — /power is the production series.
	// Load from metered import/export + reported PV/storage (15‑min averages).
	load := snap.PVWatts + importW - exportW - chargeW + dischargeW
	if load < 0 {
		load = 0
	}
	if snap.HasMeter || load > 0 {
		snap.LoadWatts = load
		snap.HasMeter = true
	}

	if ov, err := c.getOverview(ctx, token, site); err == nil {
		snap.TodayWh = ov.TodayWh
		snap.MonthWh = ov.MonthWh
		snap.LifetimeWh = ov.LifetimeWh
		if !ov.Updated.IsZero() {
			snap.Updated = ov.Updated
		}
	}
	return snap, nil
}

type powerFlowDTO struct {
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
}

func (c *OAuthClient) getPowerFlow(ctx context.Context, token, site string) (powerFlowDTO, error) {
	// Start a short live session when supported (ignore errors).
	_ = c.doJSON(ctx, http.MethodPost, "/sites/"+url.PathEscape(site)+"/power-flow", token, nil, nil)

	var wrap struct {
		SiteCurrentPowerFlow powerFlowDTO `json:"siteCurrentPowerFlow"`
		PowerFlow            powerFlowDTO `json:"powerFlow"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/sites/"+url.PathEscape(site)+"/power-flow", token, nil, &wrap); err != nil {
		// Some accounts expose the V1 path with a bearer token.
		var v1 struct {
			SiteCurrentPowerFlow powerFlowDTO `json:"siteCurrentPowerFlow"`
		}
		if err2 := c.doJSONAbs(ctx, http.MethodGet, "https://monitoringapi.solaredge.com/site/"+url.PathEscape(site)+"/currentPowerFlow", token, nil, &v1); err2 != nil {
			return powerFlowDTO{}, err
		}
		return v1.SiteCurrentPowerFlow, nil
	}
	if wrap.SiteCurrentPowerFlow.PV != nil || wrap.SiteCurrentPowerFlow.GRID != nil || wrap.SiteCurrentPowerFlow.STORAGE != nil {
		return wrap.SiteCurrentPowerFlow, nil
	}
	return wrap.PowerFlow, nil
}

func applyPowerFlow(snap *Snapshot, flow powerFlowDTO) {
	// SolarEdge power-flow CurrentPower is in kW.
	kw := func(v float64) float64 { return v * 1000 }
	if flow.PV != nil {
		snap.PVWatts = kw(flow.PV.CurrentPower)
	}
	if flow.LOAD != nil {
		snap.LoadWatts = kw(flow.LOAD.CurrentPower)
		snap.HasMeter = true
	}
	if flow.GRID != nil {
		snap.GridWatts = kw(flow.GRID.CurrentPower)
		snap.HasMeter = true
	}
	charging, discharging := false, false
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
		snap.BatteryWatts = kw(flow.STORAGE.CurrentPower)
		snap.BatteryCritical = flow.STORAGE.Critical
		snap.BatteryCharging = charging && !discharging
		if discharging && !charging {
			snap.BatteryCharging = false
		}
		st := strings.ToUpper(flow.STORAGE.Status)
		if strings.Contains(st, "CHARGE") {
			snap.BatteryCharging = true
		}
		if strings.Contains(st, "DISCHARGE") {
			snap.BatteryCharging = false
		}
	}
}

func (c *OAuthClient) getCurrentPower(ctx context.Context, token, site string) (float64, error) {
	var payload struct {
		Values []struct {
			Value *float64 `json:"value"`
		} `json:"values"`
		Power *float64 `json:"power"`
		Unit  string   `json:"unit"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/sites/"+url.PathEscape(site)+"/power", token, nil, &payload); err != nil {
		return 0, err
	}
	for i := len(payload.Values) - 1; i >= 0; i-- {
		if payload.Values[i].Value != nil {
			return *payload.Values[i].Value, nil
		}
	}
	if payload.Power != nil {
		return *payload.Power, nil
	}
	return 0, nil
}

func (c *OAuthClient) getOverview(ctx context.Context, token, site string) (Snapshot, error) {
	var payload struct {
		// V1-shaped overview (rarely returned on V2 bearer).
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
		// V2 Monitoring API overview.
		Production struct {
			Total            float64 `json:"total"`
			Unit             string  `json:"unit"`
			ToSelfConsumption float64 `json:"toSelfConsumption"`
			ToStorage        float64 `json:"toStorage"`
			ToGrid           float64 `json:"toGrid"`
		} `json:"production"`
		Consumption struct {
			Total       float64 `json:"total"`
			Unit        string  `json:"unit"`
			FromPv      float64 `json:"fromPv"`
			FromStorage float64 `json:"fromStorage"`
			FromGrid    float64 `json:"fromGrid"`
		} `json:"consumption"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/sites/"+url.PathEscape(site)+"/overview", token, nil, &payload); err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{
		PVWatts:     payload.Overview.CurrentPower.Power,
		TodayWh:     payload.Overview.LastDayData.Energy,
		MonthWh:     payload.Overview.LastMonthData.Energy,
		LifetimeWh: payload.Overview.LifeTimeData.Energy,
	}
	if snap.TodayWh == 0 && payload.Production.Total > 0 {
		snap.TodayWh = energyToWh(payload.Production.Total, payload.Production.Unit)
	}
	// Without power-flow (Advanced tier), approximate live load/grid from
	// today's overview split only when we have no better source.
	if payload.Consumption.Total > 0 {
		snap.HasMeter = true
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", payload.Overview.LastUpdateTime, time.Local); err == nil {
		snap.Updated = t
	}
	return snap, nil
}

func energyToWh(v float64, unit string) float64 {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "KWH":
		return v * 1000
	case "MWH":
		return v * 1_000_000
	default: // WH
		return v
	}
}

type telemetryPoints []struct {
	Value *float64 `json:"value"`
}

func lastTelemetry(vals telemetryPoints) *float64 {
	for i := len(vals) - 1; i >= 0; i-- {
		if vals[i].Value != nil {
			return vals[i].Value
		}
	}
	return nil
}

func (c *OAuthClient) applyStorageTelemetry(ctx context.Context, token, site string, snap *Snapshot, chargeW, dischargeW *float64) error {
	now := time.Now()
	q := url.Values{}
	// V2 wants RFC3339 with offset; space-separated timestamps 400.
	q.Set("from", now.Add(-6*time.Hour).Format(time.RFC3339))
	q.Set("to", now.Format(time.RFC3339))
	var payload struct {
		Storage map[string]struct {
			ChargePower struct {
				Values telemetryPoints `json:"values"`
			} `json:"chargePower"`
			DischargePower struct {
				Values telemetryPoints `json:"values"`
			} `json:"dischargePower"`
			StateOfEnergy struct {
				Values telemetryPoints `json:"values"`
			} `json:"stateOfEnergy"`
		} `json:"storage"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/sites/"+url.PathEscape(site)+"/storage/telemetry?"+q.Encode(), token, nil, &payload); err != nil {
		return err
	}
	for _, b := range payload.Storage {
		charge := lastTelemetry(b.ChargePower.Values)
		discharge := lastTelemetry(b.DischargePower.Values)
		soe := lastTelemetry(b.StateOfEnergy.Values)
		if charge == nil && discharge == nil && soe == nil {
			continue
		}
		snap.HasBattery = true
		var cW, dW float64
		if charge != nil {
			cW = *charge
		}
		if discharge != nil {
			dW = *discharge
		}
		if chargeW != nil {
			*chargeW = cW
		}
		if dischargeW != nil {
			*dischargeW = dW
		}
		if dW >= cW && dW > 1 {
			snap.BatteryWatts = dW
			snap.BatteryCharging = false
		} else if cW > 1 {
			snap.BatteryWatts = cW
			snap.BatteryCharging = true
		}
		if soe != nil {
			pct := *soe
			// Some sites report 0–1 fraction; others 0–100.
			if pct <= 1.5 {
				pct *= 100
			}
			snap.BatteryPct = int(pct + 0.5)
			snap.BatteryCritical = snap.BatteryPct > 0 && snap.BatteryPct < 10
		}
		return nil
	}
	return nil
}

func (c *OAuthClient) applyMeterTelemetry(ctx context.Context, token, site string, importW, exportW *float64) error {
	now := time.Now()
	q := url.Values{}
	q.Set("from", now.Add(-6*time.Hour).Format(time.RFC3339))
	q.Set("to", now.Format(time.RFC3339))
	var payload struct {
		Meters map[string]struct {
			ImportPower struct {
				Values telemetryPoints `json:"values"`
			} `json:"importPower"`
			ExportPower struct {
				Values telemetryPoints `json:"values"`
			} `json:"exportPower"`
		} `json:"meters"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/sites/"+url.PathEscape(site)+"/meters/telemetry?"+q.Encode(), token, nil, &payload); err != nil {
		return err
	}
	for _, m := range payload.Meters {
		imp := lastTelemetry(m.ImportPower.Values)
		exp := lastTelemetry(m.ExportPower.Values)
		if importW != nil && imp != nil {
			*importW = *imp
		}
		if exportW != nil && exp != nil {
			*exportW = *exp
		}
		return nil
	}
	return fmt.Errorf("solaredge: no meter telemetry")
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (c *OAuthClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	tok := c.token.AccessToken
	exp := c.token.ExpiresAt
	c.mu.Unlock()
	if tok != "" && time.Now().Before(exp.Add(-tokenExpiryMargin)) {
		return tok, nil
	}
	return c.refresh(ctx)
}

func (c *OAuthClient) refresh(ctx context.Context) (string, error) {
	rt := c.refreshToken()
	if rt == "" {
		return "", fmt.Errorf("solaredge oauth: no refresh token (run deploy/pi/solaredge-auth.ps1)")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", rt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	basic := base64.StdEncoding.EncodeToString([]byte(c.opts.ClientID + ":" + c.opts.ClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("solaredge token refresh: %s: %s", res.Status, truncate(string(body), 200))
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("solaredge token refresh: empty access_token")
	}
	exp := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if out.ExpiresIn <= 0 {
		exp = time.Now().Add(2 * time.Hour)
	}
	c.mu.Lock()
	c.token.AccessToken = out.AccessToken
	c.token.ExpiresAt = exp
	if out.RefreshToken != "" {
		c.token.RefreshToken = out.RefreshToken
	}
	if c.token.SiteID == "" {
		c.token.SiteID = c.opts.SiteID
	}
	path := c.opts.TokenPath
	tf := c.token
	c.mu.Unlock()
	_ = saveTokenFile(path, tf)
	return out.AccessToken, nil
}

func (c *OAuthClient) doJSON(ctx context.Context, method, path, token string, body io.Reader, dest any) error {
	return c.doJSONAbs(ctx, method, apiV2Base+path, token, body, dest)
}

func (c *OAuthClient) doJSONAbs(ctx context.Context, method, rawURL, token string, body io.Reader, dest any) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("solaredge %s: %s: %s", rawURL, res.Status, truncate(string(b), 200))
	}
	if dest == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dest)
}

func loadTokenFile(path string) (tokenFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return tokenFile{}, err
	}
	var t tokenFile
	if err := json.Unmarshal(b, &t); err != nil {
		return tokenFile{}, err
	}
	return t, nil
}

func saveTokenFile(path string, t tokenFile) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
