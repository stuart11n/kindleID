package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"kindle-shazam/internal/audio"
	"kindle-shazam/internal/detect"
	"kindle-shazam/internal/identify"
	"kindle-shazam/internal/listen"
	"kindle-shazam/internal/server"
	"kindle-shazam/internal/solaredge"
	"kindle-shazam/internal/weather"
)

// Site defaults — placeholders in git; deploy/pi/deploy.ps1 overrides via -ldflags
// from config.local.json.
var (
	defaultLat         = "0"
	defaultLon         = "0"
	defaultPlace       = "YOUR_CITY"
	defaultSolarModbus = "SOLAREDGE_HOST:502"
	defaultSolarUnit   = "1"
)

func main() {
	device := flag.String("device", "first", "capture device name, ALSA id (plughw:1,0), or first")
	list := flag.Bool("list-devices", false, "print capture devices and exit")
	listenAddr := flag.String("listen", ":8080", "HTTP listen address")
	width := flag.Int("width", 1448, "logical screen width (Paperwhite 7th gen landscape is 1448; portrait is 1072)")
	height := flag.Int("height", 1072, "logical screen height (Paperwhite 7th gen landscape is 1072; portrait is 1448)")
	threshold := flag.Float64("threshold", 0.005, "RMS below this counts as silence")
	settle := flag.Duration("settle", 2*time.Second, "sound required before a new track is real")
	clip := flag.Duration("clip", 5*time.Second, "first audio length sent to identify (AudD uses ~last 12s)")
	missWait := flag.Duration("miss-wait", 20*time.Second, "after a miss, keep recording this long then resubmit the whole buffer")
	fullClip := flag.Duration("full-clip", 30*time.Second, "RAM ring length kept for full resubmit after a miss")
	gap := flag.Duration("gap", 500*time.Millisecond, "silence that separates tracks")
	silence := flag.Duration("silence", 20*time.Second, "silence before Nothing playing")
	recheck := flag.Duration("recheck", time.Minute, "how often a playing record is checked again")
	lat := flag.Float64("lat", mustFloat(defaultLat), "weather latitude")
	lon := flag.Float64("lon", mustFloat(defaultLon), "weather longitude")
	place := flag.String("place", defaultPlace, "weather location label")
	celsius := flag.Bool("celsius", false, "show temperatures in Celsius")
	weatherEvery := flag.Duration("weather-every", 15*time.Minute, "how often to refresh idle weather")
	solarEvery := flag.Duration("solar-every", time.Hour, "how often to refresh solar stats daytime (skips 9pm–6am; 1h fits free-tier with meters+storage)")
	solarAddr := flag.String("solar-modbus", envOr("SOLAREDGE_MODBUS", defaultSolarModbus), "SolarEdge Modbus TCP host:port (fallback if API unset)")
	solarUnit := flag.Uint("solar-unit", mustUint(defaultSolarUnit), "SolarEdge Modbus unit id")
	seClientID := flag.String("solaredge-client-id", envOr("SOLAREDGE_CLIENT_ID", ""), "SolarEdge OAuth client id")
	seClientSecret := flag.String("solaredge-client-secret", envOr("SOLAREDGE_CLIENT_SECRET", ""), "SolarEdge OAuth client secret")
	seSiteID := flag.String("solaredge-site-id", envOr("SOLAREDGE_SITE_ID", ""), "SolarEdge site id")
	seRefresh := flag.String("solaredge-refresh-token", envOr("SOLAREDGE_REFRESH_TOKEN", ""), "SolarEdge OAuth refresh token (seed; rotated tokens persist on disk)")
	seTokenPath := flag.String("solaredge-token-path", envOr("SOLAREDGE_TOKEN_PATH", solaredge.DefaultTokenPath()), "path for SolarEdge OAuth token JSON")
	radar := flag.Bool("radar", false, "cycle the right idle panel with a 100mi radar map")
	identifyProvider := flag.String("identify", envOr("IDENTIFY_PROVIDER", "both"), "music ID backend: both (race AudD+Shazam), audd, or shazam")
	auddToken := flag.String("audd-token", envOr("AUDD_API_TOKEN", ""), "AudD API token (or set AUDD_API_TOKEN)")
	settingsPath := flag.String("settings", envOr("KINDLE_SHAZAM_SETTINGS", server.DefaultSettingsPath()), "runtime settings JSON path")
	flag.Parse()

	log.SetFlags(log.Ltime)

	if *list {
		devices, err := audio.List()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(audio.FormatDevices(devices))
		return
	}

	base := server.SettingsValues{
		Identify:     *identifyProvider,
		Threshold:    *threshold,
		Settle:       server.FormatDuration(*settle),
		Clip:         server.FormatDuration(*clip),
		MissWait:     server.FormatDuration(*missWait),
		FullClip:     server.FormatDuration(*fullClip),
		Gap:          server.FormatDuration(*gap),
		Silence:      server.FormatDuration(*silence),
		Recheck:      server.FormatDuration(*recheck),
		WeatherEvery: server.FormatDuration(*weatherEvery),
		SolarEvery:   server.FormatDuration(*solarEvery),
		SolarSource:  "api",
		SolarModbus:  *solarAddr,
		SolarUnit:    int(*solarUnit),
		Celsius:      *celsius,
		Radar:        *radar,
		Lat:          *lat,
		Lon:          *lon,
		Place:        *place,
		Width:        *width,
		Height:       *height,
		AudDToken:    *auddToken,
	}
	settings, err := server.NewSettings(*settingsPath, base)
	if err != nil {
		log.Fatal(err)
	}
	v := settings.GetRaw()

	display := server.NewDisplay(*device, v.Threshold)
	display.SetSettings(settings)
	// Keep the last 50 lines in RAM only (/log). Do not write to stderr/journal.
	log.SetOutput(display.Logs())

	idClient, err := identify.New(identify.Options{
		Provider:  v.Identify,
		AudDToken: v.AudDToken,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("identify %s", idClient.Name())
	log.Printf("settings %s", settings.Path())

	wx := weather.New(weather.Options{
		Lat:     v.Lat,
		Lon:     v.Lon,
		Place:   v.Place,
		Celsius: v.Celsius,
	})
	api := solaredge.NewOAuth(solaredge.OAuthOptions{
		ClientID:     *seClientID,
		ClientSecret: *seClientSecret,
		SiteID:       *seSiteID,
		RefreshToken: *seRefresh,
		TokenPath:    *seTokenPath,
	})
	display.SetSolarOAuth(api)
	mbAddr := v.SolarModbus
	if mbAddr == "" {
		mbAddr = *solarAddr
	}
	mbUnit := byte(v.SolarUnit)
	if mbUnit == 0 {
		mbUnit = byte(*solarUnit)
	}
	mb := solaredge.NewModbus(solaredge.ModbusOptions{Addr: mbAddr, UnitID: mbUnit})
	mode := v.SolarSource
	if mode == "" {
		mode = solaredge.ModeAPI
	}
	solarSwitch := solaredge.NewSwitchable(mode, api, mb)
	se := solaredge.Source(solarSwitch)
	switch solarSwitch.Mode() {
	case solaredge.ModeModbus:
		if mb.Enabled() {
			log.Printf("solaredge modbus %s unit %d", mbAddr, mbUnit)
		} else {
			log.Printf("solaredge modbus: set host in /settings")
		}
	default:
		if api.Enabled() {
			log.Printf("solaredge api site %s (oauth)", api.Status().SiteID)
		} else if api.Configured() {
			log.Printf("solaredge api: authorize via /settings")
		} else {
			log.Printf("solaredge api: set SOLAREDGE_CLIENT_ID/SECRET on the Pi")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- listen.Run(ctx, listen.Options{
			Device:       *device,
			Display:      display,
			Client:       idClient,
			Width:        v.Width,
			Height:       v.Height,
			Weather:      wx,
			SolarEdge:    se,
			SolarSwitch:  solarSwitch,
			Refresh:      server.MustDuration(v.WeatherEvery, 15*time.Minute),
			SolarRefresh: server.MustDuration(v.SolarEvery, time.Hour),
			Radar:        v.Radar,
			Settings:     settings,
			Detect: detect.Config{
				SampleRate: 16000,
				Threshold:  v.Threshold,
				Settle:     server.MustDuration(v.Settle, 2*time.Second),
				Clip:       server.MustDuration(v.Clip, 5*time.Second),
				MissWait:   server.MustDuration(v.MissWait, 20*time.Second),
				FullClip:   server.MustDuration(v.FullClip, 30*time.Second),
				Gap:        server.MustDuration(v.Gap, 500*time.Millisecond),
				Silence:    server.MustDuration(v.Silence, 20*time.Second),
				Recheck:    server.MustDuration(v.Recheck, time.Minute),
			},
		})
	}()

	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           display.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("http %s", *listenAddr)
		for _, url := range kindleURLs(*listenAddr) {
			log.Printf("kindle %s", url)
			log.Printf("settings %s/settings", url)
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Fatal(err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func mustFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func mustUint(s string) uint {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil || v == 0 {
		return 1
	}
	return uint(v)
}

func kindleURLs(listenAddr string) []string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return []string{"http://" + listenAddr}
	}
	if port == "" {
		port = "8080"
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{"http://" + net.JoinHostPort(host, port)}
	}
	ip := outboundIP()
	if ip == "" {
		return []string{"http://127.0.0.1:" + port}
	}
	return []string{"http://" + net.JoinHostPort(ip, port)}
}

func outboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return addr.IP.String()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
