package listen

import (
	"context"
	"errors"
	"image"
	"log"
	"strconv"
	"strings"
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
	Device           string
	Display          *server.Display
	Detect           detect.Config
	Width            int
	Height           int
	Client           *identify.Client
	Weather          *weather.Client
	SolarEdge        solaredge.Source
	SolarSwitch      *solaredge.Switchable // optional; enables api/modbus hot-swap
	Refresh          time.Duration
	SolarRefresh     time.Duration
	SatelliteRefresh time.Duration
	PanelCycle       time.Duration
	Radar            bool // cycle right panel with radar map when true
	Settings         *server.Settings
}

// live holds settings that /settings can change without restarting capture.
type live struct {
	mu           sync.RWMutex
	client       *identify.Client
	detect       detect.Config
	width        int
	height       int
	refresh      time.Duration
	solarRefresh time.Duration
	radar        bool
}

func (l *live) clientOr(fallback *identify.Client) *identify.Client {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.client != nil {
		return l.client
	}
	return fallback
}

func (l *live) size() (int, int) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.width, l.height
}

func (l *live) detectCfg() detect.Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.detect
}

func (l *live) flags() (refresh, solar time.Duration, radar bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.refresh, l.solarRefresh, l.radar
}

// Run captures audio until ctx is cancelled.
func Run(ctx context.Context, opts Options) error {
	stream, err := audio.Open(opts.Device)
	if err != nil {
		return err
	}
	defer stream.Close()

	if opts.Client == nil {
		return errors.New("listen: identify client is required")
	}
	if opts.Refresh <= 0 {
		opts.Refresh = 15 * time.Minute
	}
	if opts.SolarRefresh <= 0 {
		opts.SolarRefresh = time.Hour
	}
	if opts.SatelliteRefresh <= 0 {
		opts.SatelliteRefresh = 10 * time.Minute
	}
	if opts.PanelCycle <= 0 {
		opts.PanelCycle = 5 * time.Second
	}
	machine := detect.New(opts.Detect)
	log.Printf("capture %s at %d Hz", stream.Name(), stream.SampleRate())
	opts.Display.SetDevice(stream.Name())

	lv := &live{
		client:       opts.Client,
		detect:       opts.Detect,
		width:        opts.Width,
		height:       opts.Height,
		refresh:      opts.Refresh,
		solarRefresh: opts.SolarRefresh,
		radar:        opts.Radar,
	}

	var mu sync.Mutex
	var wx weather.Snapshot
	var haveWX bool
	var solar solaredge.Snapshot
	var haveSolar bool
	var sat image.Image
	var showSat bool

	showIdle := func(force bool) {
		w, h := lv.size()
		_, _, radar := lv.flags()
		mu.Lock()
		idle := render.Idle{
			Satellite:     sat,
			ShowSatellite: radar && showSat && sat != nil,
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

		png, err := render.IdlePNG(w, h, idle)
		if err != nil {
			log.Printf("render idle: %v", err)
			return
		}
		if opts.Display.Set(png, server.Meta{}) || force {
			switch {
			case idle.Weather != nil && idle.ShowSatellite:
				log.Printf("screen: weather %s %d%s + radar", idle.Weather.Place, idle.Weather.Temp, idle.Weather.Unit)
			case idle.Weather != nil && idle.Solar != nil:
				extra := " solar " + solaredge.FormatPower(idle.Solar.PVWatts)
				if idle.Solar.HasMeter {
					dir := "import"
					if idle.Solar.GridExport {
						dir = "export"
					}
					extra += " grid " + solaredge.FormatPower(idle.Solar.GridWatts) + " " + dir
					extra += " load " + solaredge.FormatPower(idle.Solar.LoadWatts)
				}
				if idle.Solar.HasBattery {
					extra += fmtBattery(idle.Solar.BatteryPct)
					if idle.Solar.BatteryWatts > 25 {
						if idle.Solar.BatteryCharging {
							extra += " chg " + solaredge.FormatPower(idle.Solar.BatteryWatts)
						} else {
							extra += " dis " + solaredge.FormatPower(idle.Solar.BatteryWatts)
						}
					}
				}
				log.Printf("screen: weather %s %d%s +%s", idle.Weather.Place, idle.Weather.Temp, idle.Weather.Unit, extra)
			case idle.Weather != nil:
				log.Printf("screen: weather %s %d%s %s", idle.Weather.Place, idle.Weather.Temp, idle.Weather.Unit, idle.Weather.Condition)
			case idle.Solar != nil:
				log.Printf("screen: solar %s", solaredge.FormatPower(idle.Solar.PVWatts))
			default:
				log.Printf("screen: Nothing playing.")
			}
		}
	}

	paintTrack := func(card render.Card, meta server.Meta, quiet bool) {
		w, h := lv.size()
		png, err := render.PNG(w, h, card)
		if err != nil {
			log.Printf("render: %v", err)
			return
		}
		if !opts.Display.Set(png, meta) {
			return
		}
		if quiet {
			return
		}
		if card.Art != nil {
			log.Printf("screen: %s — %s (+art)", meta.Artist, meta.Title)
			return
		}
		if meta.Year != "" {
			log.Printf("screen: %s — %s (%s · %s)", meta.Artist, meta.Title, meta.Album, meta.Year)
		} else {
			log.Printf("screen: %s — %s (%s)", meta.Artist, meta.Title, meta.Album)
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
		_, _, radarOn := lv.flags()
		if !radarOn || opts.Weather == nil {
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

	intervalKick := make(chan struct{}, 1)

	if opts.Settings != nil {
		opts.Settings.OnApply(func(v server.SettingsValues) error {
			cfg := detect.Config{
				SampleRate: 16000,
				Threshold:  v.Threshold,
				Settle:     server.MustDuration(v.Settle, 2*time.Second),
				Clip:       server.MustDuration(v.Clip, 5*time.Second),
				MissWait:   server.MustDuration(v.MissWait, 20*time.Second),
				FullClip:   server.MustDuration(v.FullClip, 30*time.Second),
				Gap:        server.MustDuration(v.Gap, 500*time.Millisecond),
				Silence:    server.MustDuration(v.Silence, 20*time.Second),
				Recheck:    server.MustDuration(v.Recheck, time.Minute),
			}
			machine.SetConfig(cfg)
			client, err := identify.New(identify.Options{
				Provider:  v.Identify,
				AudDToken: v.AudDToken,
			})
			if err != nil {
				return err
			}
			if opts.Weather != nil {
				opts.Weather.SetOptions(weather.Options{
					Lat:     v.Lat,
					Lon:     v.Lon,
					Place:   v.Place,
					Celsius: v.Celsius,
				})
			}
			lv.mu.Lock()
			lv.client = client
			lv.detect = cfg
			lv.width = v.Width
			lv.height = v.Height
			lv.refresh = server.MustDuration(v.WeatherEvery, 15*time.Minute)
			solarEvery := server.MustDuration(v.SolarEvery, time.Hour)
			mode := v.SolarSource
			if mode == "" {
				mode = solaredge.ModeAPI
			}
			// SolarEdge Modbus TCP is fragile — never poll faster than 15m.
			if mode == solaredge.ModeModbus && solarEvery < 15*time.Minute {
				solarEvery = 15 * time.Minute
			}
			lv.solarRefresh = solarEvery
			lv.radar = v.Radar
			lv.mu.Unlock()
			if opts.SolarSwitch != nil {
				opts.SolarSwitch.SetMode(mode)
				if addr := strings.TrimSpace(v.SolarModbus); addr != "" {
					unit := byte(v.SolarUnit)
					if unit == 0 {
						unit = 1
					}
					opts.SolarSwitch.SetModbus(addr, unit)
				}
				opts.SolarEdge = opts.SolarSwitch
			}
			opts.Display.Observe(machine.State(), machine.RMS())

			// Patch the cached idle snapshot and redraw now (don't wait for next fetch).
			mu.Lock()
			if haveWX {
				wx.Place = v.Place
				if v.Celsius {
					wx.Unit = "°C"
				} else {
					wx.Unit = "°F"
				}
			}
			mu.Unlock()
			if opts.Display.Idle() {
				showIdle(true)
			}
			go func() { _ = refreshWeather() }()
			go refreshSolar()
			select {
			case intervalKick <- struct{}{}:
			default:
			}
			src := v.SolarSource
			if src == "" {
				src = "api"
			}
			log.Printf("settings applied (identify %s, place %q, solar %s every %s)", client.Name(), v.Place, src, v.SolarEvery)
			return nil
		})
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
	if !solarQuietHours(time.Now()) {
		refreshSolar()
	} else {
		log.Printf("solaredge: quiet hours (9pm–6am); next poll at 6am")
	}
	if opts.Radar {
		refreshSatellite()
	}
	if opts.Display.Idle() {
		showIdle(true)
	}

	go func() {
		weatherDur, solarDur, _ := lv.flags()
		weatherTimer := time.NewTimer(weatherDur)
		solarTimer := time.NewTimer(nextSolarDelay(time.Now(), solarDur))
		defer weatherTimer.Stop()
		defer solarTimer.Stop()
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

		resetTimers := func() {
			wd, sd, _ := lv.flags()
			if !weatherTimer.Stop() {
				select {
				case <-weatherTimer.C:
				default:
				}
			}
			if !solarTimer.Stop() {
				select {
				case <-solarTimer.C:
				default:
				}
			}
			weatherTimer.Reset(wd)
			solarTimer.Reset(nextSolarDelay(time.Now(), sd))
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-intervalKick:
				resetTimers()
			case <-weatherTimer.C:
				if !refreshWeather() {
					weatherRetry = time.After(30 * time.Second)
				} else {
					weatherRetry = nil
				}
				wd, _, _ := lv.flags()
				weatherTimer.Reset(wd)
			case <-weatherRetry:
				if !refreshWeather() {
					weatherRetry = time.After(30 * time.Second)
				} else {
					weatherRetry = nil
				}
			case <-solarTimer.C:
				now := time.Now()
				if !solarQuietHours(now) {
					refreshSolar()
				}
				_, sd, _ := lv.flags()
				solarTimer.Reset(nextSolarDelay(time.Now(), sd))
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

	var lastState string
	for {
		select {
		case <-ctx.Done():
			return nil
		case chunk, ok := <-stream.Chunks():
			if !ok {
				return errors.New("audio stream closed")
			}
			act := machine.Push(chunk)
			state := machine.State()
			opts.Display.Observe(state, machine.RMS())
			if state != lastState {
				lastState = state
				log.Printf("%s rms=%.4f threshold=%.4f", state, machine.RMS(), lv.detectCfg().Threshold)
			}
			if act.ShowNothing {
				showIdle(false)
			}
			if act.Identify {
				clip := append([]float32(nil), act.Clip...)
				gen := act.Generation
				reason := act.Reason
				go identifyClip(lv, opts, machine, clip, gen, reason, paintTrack, func() {
					machine.ForgetTrack()
					showIdle(true)
				})
			}
		}
	}
}

func fmtBattery(pct int) string {
	return " batt " + strconv.Itoa(pct) + "%"
}

func identifyClip(lv *live, opts Options, machine *detect.Machine, clip []float32, gen int, reason string, paint func(render.Card, server.Meta, bool), clearStale func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := lv.clientOr(opts.Client)
	det := lv.detectCfg()
	track, err := client.Identify(ctx, clip)
	if err != nil {
		log.Printf("identify %s: %v", reason, err)
	}
	if !machine.Finish(gen, err == nil && track != nil) {
		log.Printf("dropped stale %s result", reason)
		return
	}
	if err != nil || track == nil {
		if opts.Display != nil && opts.Display.Failures() != nil {
			opts.Display.Failures().Add(clip, 16000, reason)
		}
		wait := machine.ScheduleMissRecheck()
		log.Printf("identify %s: no match; retry in %s", reason, wait.Round(time.Second))
		// Don't leave the previous track on screen across a new song / hard miss streak.
		// Clear wrong artwork on a new-track miss, or after a long miss streak.
		if clearStale != nil && (reason == "start" || machine.MissFails() == 3 || machine.MissFails() == 8) {
			log.Printf("clearing stale track after %s miss (%d)", reason, machine.MissFails())
			clearStale()
		}
		return
	}

	clipDur := det.Clip
	if reason == "full" {
		clipDur = time.Duration(len(clip)) * time.Second / 16000
	}
	if clipDur <= 0 {
		clipDur = 10 * time.Second
	}
	wait := detect.NextRecheck(track.Duration, track.Offset, clipDur, det.Recheck, 15*time.Second)
	machine.ScheduleRecheck(wait)
	via := track.Provider
	if via == "" {
		via = client.Name()
	}
	if track.Duration > 0 {
		log.Printf("recheck in %s via %s (track %s, offset %s)", wait.Round(time.Second), via, track.Duration.Round(time.Second), track.Offset.Round(time.Second))
	} else {
		log.Printf("recheck in %s via %s (no duration)", wait.Round(time.Second), via)
	}

	if opts.Display.SameTrack(track.Key) {
		log.Printf("still %s — %s", track.Artist, track.Title)
		return
	}
	meta := server.Meta{
		Key:    track.Key,
		Title:  track.Title,
		Artist: track.Artist,
		Album:  track.Album,
		Year:   track.Year,
	}
	card := render.Card{
		Title:  track.Title,
		Artist: track.Artist,
		Album:  track.Album,
		Year:   track.Year,
	}
	if paint == nil {
		return
	}
	// Reuse cover from the 50 MiB FIFO cache (album and/or art URL keys).
	if art := opts.Display.CoverArt(track.Artist, track.Album, track.ArtURL); art != nil {
		log.Printf("cover art: cache hit (%s / %s)", track.Artist, track.Album)
		card.Art = art
		paint(card, meta, false)
		return
	}
	paint(card, meta, false)
	if track.ArtURL == "" {
		log.Printf("cover art: no url for %s / %s", track.Artist, track.Album)
		return
	}
	log.Printf("cover art: cache miss; fetching")
	img, err := client.DownloadArt(ctx, track.ArtURL)
	if err != nil {
		log.Printf("cover art: %v", err)
		return
	}
	if img == nil {
		return
	}
	// Cache even if the screen moved on — next track on this album can reuse it.
	opts.Display.RememberCover(track.Artist, track.Album, track.ArtURL, img)
	if !opts.Display.SameTrack(track.Key) {
		return
	}
	card.Art = img
	paint(card, meta, false)
}

// solarQuietHours is 21:00–06:00 local — no SolarEdge API polls (free-tier credits).
func solarQuietHours(now time.Time) bool {
	h := now.Hour()
	return h >= 21 || h < 6
}

// nextSolarDelay returns the daytime poll interval, or sleep until 06:00 during quiet hours.
func nextSolarDelay(now time.Time, every time.Duration) time.Duration {
	if !solarQuietHours(now) {
		if every <= 0 {
			every = time.Hour
		}
		return every
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, now.Location())
	if now.Hour() >= 21 {
		next = next.Add(24 * time.Hour)
	}
	d := next.Sub(now)
	if d < time.Second {
		return time.Second
	}
	return d
}
