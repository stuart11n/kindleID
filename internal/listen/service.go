package listen

import (
	"context"
	"errors"
	"image"
	"log"
	"strconv"
	"sync"
	"time"

	"kindle-shazam/internal/audio"
	"kindle-shazam/internal/detect"
	"kindle-shazam/internal/identify"
	"kindle-shazam/internal/render"
	"kindle-shazam/internal/server"
	"kindle-shazam/internal/solaredge"
	"kindle-shazam/internal/weather"
)

// Options is the listener's runtime configuration.
type Options struct {
	Device    string
	Display   *server.Display
	Detect    detect.Config
	Width     int
	Height    int
	Client    *identify.Client
	Weather          *weather.Client
	SolarEdge        solaredge.Source
	Refresh          time.Duration
	SolarRefresh     time.Duration
	SatelliteRefresh time.Duration
	PanelCycle       time.Duration
	Radar            bool // cycle right panel with radar map when true
}

// Run captures audio until ctx is cancelled.
func Run(ctx context.Context, opts Options) error {
	stream, err := audio.Open(opts.Device)
	if err != nil {
		return err
	}
	defer stream.Close()

	if opts.Client == nil {
		opts.Client = identify.New()
	}
	if opts.Refresh <= 0 {
		opts.Refresh = 15 * time.Minute
	}
	if opts.SolarRefresh <= 0 {
		opts.SolarRefresh = time.Minute
	}
	if opts.Radar {
		if opts.SatelliteRefresh <= 0 {
			opts.SatelliteRefresh = 10 * time.Minute
		}
		if opts.PanelCycle <= 0 {
			opts.PanelCycle = 5 * time.Second
		}
	}
	machine := detect.New(opts.Detect)
	log.Printf("capture %s at %d Hz", stream.Name(), stream.SampleRate())
	opts.Display.SetDevice(stream.Name())

	var mu sync.Mutex
	var wx weather.Snapshot
	var haveWX bool
	var solar solaredge.Snapshot
	var haveSolar bool
	var sat image.Image
	var showSat bool

	showIdle := func(force bool) {
		mu.Lock()
		idle := render.Idle{
			Satellite:     sat,
			ShowSatellite: showSat && sat != nil,
		}
		if haveWX {
			s := wx
			idle.Weather = &s
		}
		if haveSolar {
			s := solar
			idle.Solar = &s
		}
		mu.Unlock()

		png, err := render.IdlePNG(opts.Width, opts.Height, idle)
		if err != nil {
			log.Printf("render idle: %v", err)
			return
		}
		if opts.Display.Set(png, server.Meta{}) || force {
			switch {
			case idle.Weather != nil && idle.ShowSatellite:
				log.Printf("screen: weather %s %d%s + radar", idle.Weather.Place, idle.Weather.Temp, idle.Weather.Unit)
			case idle.Weather != nil && idle.Solar != nil:
				batt := ""
				if idle.Solar.HasBattery {
					batt = fmtBattery(idle.Solar.BatteryPct)
				}
				log.Printf("screen: weather %s %d%s + solar %s%s", idle.Weather.Place, idle.Weather.Temp, idle.Weather.Unit, solaredge.FormatPower(idle.Solar.PVWatts), batt)
			case idle.Weather != nil:
				log.Printf("screen: weather %s %d%s %s", idle.Weather.Place, idle.Weather.Temp, idle.Weather.Unit, idle.Weather.Condition)
			case idle.Solar != nil:
				log.Printf("screen: solar %s", solaredge.FormatPower(idle.Solar.PVWatts))
			default:
				log.Printf("screen: Nothing playing.")
			}
		}
	}

	refreshWeather := func() bool {
		if opts.Weather == nil {
			return false
		}
		wctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		next, err := opts.Weather.Fetch(wctx)
		cancel()
		if err != nil {
			log.Printf("weather: %v", err)
			return false
		}
		mu.Lock()
		wx = next
		haveWX = true
		mu.Unlock()
		if opts.Display.Idle() {
			showIdle(false)
		}
		return true
	}

	refreshSolar := func() {
		if opts.SolarEdge == nil || !opts.SolarEdge.Enabled() {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		next, err := opts.SolarEdge.Fetch(sctx)
		cancel()
		if err != nil {
			log.Printf("solaredge: %v", err)
			return
		}
		mu.Lock()
		solar = next
		haveSolar = true
		mu.Unlock()
		if opts.Display.Idle() {
			showIdle(false)
		}
	}

	refreshSatellite := func() {
		if !opts.Radar || opts.Weather == nil {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		next, err := opts.Weather.FetchSatellite(sctx)
		cancel()
		if err != nil {
			log.Printf("radar: %v", err)
			return
		}
		mu.Lock()
		sat = next
		mu.Unlock()
		log.Printf("radar: 100mi map refreshed")
		if opts.Display.Idle() {
			showIdle(false)
		}
	}

	// Weather on a Pi 1 can miss the first TLS attempt; retry before giving up
	// for the long refresh interval.
	for attempt := 1; attempt <= 5; attempt++ {
		if refreshWeather() {
			break
		}
		if attempt == 5 {
			break
		}
		log.Printf("weather: retry %d/5 in 20s", attempt+1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Second):
		}
	}
	refreshSolar()
	if opts.Radar {
		refreshSatellite()
	}
	if opts.Display.Idle() {
		showIdle(true)
	}

	go func() {
		wt := time.NewTicker(opts.Refresh)
		st := time.NewTicker(opts.SolarRefresh)
		defer wt.Stop()
		defer st.Stop()
		var weatherRetry <-chan time.Time

		var satC <-chan time.Time
		var cycleC <-chan time.Time
		if opts.Radar {
			satT := time.NewTicker(opts.SatelliteRefresh)
			cycle := time.NewTicker(opts.PanelCycle)
			defer satT.Stop()
			defer cycle.Stop()
			satC = satT.C
			cycleC = cycle.C
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-wt.C:
				if !refreshWeather() {
					weatherRetry = time.After(30 * time.Second)
				} else {
					weatherRetry = nil
				}
			case <-weatherRetry:
				if !refreshWeather() {
					weatherRetry = time.After(30 * time.Second)
				} else {
					weatherRetry = nil
				}
			case <-st.C:
				refreshSolar()
			case <-satC:
				refreshSatellite()
			case <-cycleC:
				mu.Lock()
				canCycle := sat != nil && haveSolar
				if canCycle {
					showSat = !showSat
				}
				mu.Unlock()
				if canCycle && opts.Display.Idle() {
					showIdle(true)
				}
			}
		}
	}()

	var sinceLog int
	for {
		select {
		case <-ctx.Done():
			return nil
		case chunk, ok := <-stream.Chunks():
			if !ok {
				return errors.New("audio stream closed")
			}
			act := machine.Push(chunk)
			opts.Display.Observe(machine.State(), machine.RMS())
			sinceLog += len(chunk)
			if sinceLog >= stream.SampleRate()*2 {
				sinceLog = 0
				log.Printf("%s rms=%.4f threshold=%.4f", machine.State(), machine.RMS(), opts.Detect.Threshold)
			}
			if act.ShowNothing {
				showIdle(false)
			}
			if act.Identify {
				clip := append([]float32(nil), act.Clip...)
				gen := act.Generation
				reason := act.Reason
				go identifyClip(opts, machine, clip, gen, reason)
			}
		}
	}
}

func fmtBattery(pct int) string {
	return " batt " + strconv.Itoa(pct) + "%"
}

func identifyClip(opts Options, machine *detect.Machine, clip []float32, gen int, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	track, err := opts.Client.Identify(ctx, clip)
	if err != nil {
		log.Printf("shazam %s: %v", reason, err)
	}
	if !machine.Finish(gen, err == nil && track != nil) {
		log.Printf("dropped stale %s result", reason)
		return
	}
	if err != nil || track == nil {
		wait := machine.ScheduleMissRecheck()
		log.Printf("shazam %s: no match; retry in %s", reason, wait.Round(time.Second))
		return
	}

	clipDur := opts.Detect.Clip
	if clipDur <= 0 {
		clipDur = 10 * time.Second
	}
	wait := detect.NextRecheck(track.Duration, track.Offset, clipDur, opts.Detect.Recheck, 15*time.Second)
	machine.ScheduleRecheck(wait)
	if track.Duration > 0 {
		log.Printf("recheck in %s (track %s, offset %s)", wait.Round(time.Second), track.Duration.Round(time.Second), track.Offset.Round(time.Second))
	} else {
		log.Printf("recheck in %s (no duration)", wait.Round(time.Second))
	}

	if opts.Display.SameTrack(track.Key) {
		log.Printf("still %s — %s", track.Artist, track.Title)
		return
	}
	var art image.Image
	if img, err := opts.Client.DownloadArt(ctx, track.ArtURL); err != nil {
		log.Printf("cover art: %v", err)
	} else {
		art = img
	}
	png, err := render.PNG(opts.Width, opts.Height, render.Card{
		Title:  track.Title,
		Artist: track.Artist,
		Album:  track.Album,
		Year:   track.Year,
		Art:    art,
	})
	if err != nil {
		log.Printf("render: %v", err)
		return
	}
	if opts.Display.Set(png, server.Meta{
		Key:    track.Key,
		Title:  track.Title,
		Artist: track.Artist,
		Album:  track.Album,
		Year:   track.Year,
	}) {
		if track.Year != "" {
			log.Printf("screen: %s — %s (%s · %s)", track.Artist, track.Title, track.Album, track.Year)
		} else {
			log.Printf("screen: %s — %s (%s)", track.Artist, track.Title, track.Album)
		}
	}
}
