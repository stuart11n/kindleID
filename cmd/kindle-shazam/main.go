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
	settle := flag.Duration("settle", 6*time.Second, "sound required before a new track is real")
	clip := flag.Duration("clip", 10*time.Second, "audio sent to Shazam")
	gap := flag.Duration("gap", 2*time.Second, "silence that separates tracks")
	silence := flag.Duration("silence", 20*time.Second, "silence before Nothing playing")
	recheck := flag.Duration("recheck", time.Minute, "how often a playing record is checked again")
	lat := flag.Float64("lat", mustFloat(defaultLat), "weather latitude")
	lon := flag.Float64("lon", mustFloat(defaultLon), "weather longitude")
	place := flag.String("place", defaultPlace, "weather location label")
	celsius := flag.Bool("celsius", false, "show temperatures in Celsius")
	weatherEvery := flag.Duration("weather-every", 15*time.Minute, "how often to refresh idle weather")
	solarEvery := flag.Duration("solar-every", time.Minute, "how often to refresh Modbus solar stats")
	solarAddr := flag.String("solar-modbus", envOr("SOLAREDGE_MODBUS", defaultSolarModbus), "SolarEdge Modbus TCP host:port")
	solarUnit := flag.Uint("solar-unit", mustUint(defaultSolarUnit), "SolarEdge Modbus unit id")
	radar := flag.Bool("radar", false, "cycle the right idle panel with a 100mi radar map")
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

	display := server.NewDisplay(*device, *threshold)
	// Keep the last 50 lines in RAM only (/log). Do not write to stderr/journal.
	log.SetOutput(display.Logs())
	wx := weather.New(weather.Options{
		Lat:     *lat,
		Lon:     *lon,
		Place:   *place,
		Celsius: *celsius,
	})
	se := solaredge.NewModbus(solaredge.ModbusOptions{
		Addr:   *solarAddr,
		UnitID: byte(*solarUnit),
	})
	if se.Enabled() {
		log.Printf("solaredge modbus %s unit %d", *solarAddr, *solarUnit)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- listen.Run(ctx, listen.Options{
			Device:       *device,
			Display:      display,
			Width:        *width,
			Height:       *height,
			Weather:      wx,
			SolarEdge:    se,
			Refresh:      *weatherEvery,
			SolarRefresh: *solarEvery,
			Radar:        *radar,
			Detect: detect.Config{
				SampleRate: 16000,
				Threshold:  *threshold,
				Settle:     *settle,
				Clip:       *clip,
				Gap:        *gap,
				Silence:    *silence,
				Recheck:    *recheck,
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
