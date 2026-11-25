package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

func (d *Display) handleSettings(w http.ResponseWriter, r *http.Request) {
	if d.settings == nil {
		http.Error(w, "settings unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case "/settings.json":
		d.handleSettingsJSON(w, r)
		return
	case "/settings":
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(settingsHTML))
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	default:
		http.NotFound(w, r)
	}
}

func (d *Display) handleSettingsRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"ok":true,"restarting":true}`))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	// systemd Restart=always brings the process back after a clean exit.
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()
}

func (d *Display) handleKindleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.RequestKindleStop()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"ok":true,"kindle":"stop"}`))
}

func (d *Display) handleKindleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.ClearKindleStop()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"ok":true,"kindle":"run"}`))
}

func (d *Display) handleSettingsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		type view struct {
			SettingsValues
			HasAudDToken bool   `json:"has_audd_token"`
			Path         string `json:"path"`
		}
		v := view{
			SettingsValues: d.settings.Get(),
			HasAudDToken:   d.settings.HasAudDToken(),
			Path:           d.settings.Path(),
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
	case http.MethodPost, http.MethodPut:
		var v SettingsValues
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			http.Error(w, "expected application/json", http.StatusBadRequest)
			return
		}
		if err := d.settings.Save(v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":             true,
			"has_audd_token": d.settings.HasAudDToken(),
			"settings":       d.settings.Get(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

const settingsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kindle Shazam settings</title>
<style>
  :root { color-scheme: light; --bg:#f4f1ea; --ink:#1a1a1a; --muted:#5c5c5c; --line:#cfc8ba; --accent:#2f5d50; --card:#fffcf7; }
  * { box-sizing: border-box; }
  body { margin:0; font: 15px/1.45 "Segoe UI", system-ui, sans-serif; background: var(--bg); color: var(--ink); }
  main { max-width: 720px; margin: 0 auto; padding: 28px 18px 64px; }
  h1 { font-size: 1.6rem; margin: 0 0 6px; letter-spacing: -0.02em; }
  .sub { color: var(--muted); margin: 0 0 24px; }
  form { background: var(--card); border: 1px solid var(--line); border-radius: 12px; padding: 18px; display: grid; gap: 14px; }
  fieldset { border: 1px solid var(--line); border-radius: 10px; padding: 12px 14px 14px; margin: 0; }
  legend { padding: 0 6px; font-weight: 600; }
  .row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  label { display: grid; gap: 4px; font-size: 0.92rem; }
  label span { color: var(--muted); font-size: 0.8rem; }
  select, input[type=text], input[type=number], input[type=password] {
    width: 100%; padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px;
    background: #fff; font: inherit; color: inherit;
  }
  .actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-top: 4px; }
  button { background: var(--accent); color: #fff; border: 0; border-radius: 8px; padding: 10px 16px; font: inherit; cursor: pointer; }
  button.secondary { background: transparent; color: var(--ink); border: 1px solid var(--line); }
  button:disabled { opacity: 0.6; cursor: default; }
  .msg { min-height: 1.2em; color: var(--muted); }
  .msg.ok { color: #1b6b3a; }
  .msg.err { color: #8b1e1e; }
  a { color: var(--accent); }
  @media (max-width: 640px) { .row { grid-template-columns: 1fr; } }
</style>
</head>
<body>
<main>
  <h1>Server settings</h1>
  <p class="sub">Changes apply immediately and are saved to disk. <a href="/status">status</a> · <a href="/log?follow=0">log</a> · <a href="/match-failures">failures</a></p>
  <p class="sub">If KUAL is unreachable (Now Playing took over the screen), use <strong>Restore Kindle UI</strong> below, or hold the Kindle power button ~20–40s to reboot.</p>
  <form id="f">
    <fieldset>
      <legend>Identify</legend>
      <div class="row">
        <label>Provider
          <select name="identify" id="identify">
            <option value="shazam">shazam</option>
            <option value="audd">audd</option>
            <option value="both">both (race)</option>
          </select>
        </label>
        <label>AudD API token <span id="tokenHint">leave blank to keep</span>
          <input type="password" name="audd_api_token" id="audd_api_token" placeholder="••••••••" autocomplete="off">
        </label>
      </div>
    </fieldset>

    <fieldset>
      <legend>Detection</legend>
      <div class="row">
        <label>Silence threshold (RMS)
          <select name="threshold" id="threshold">
            <option value="0.001">0.001</option>
            <option value="0.002">0.002</option>
            <option value="0.005">0.005</option>
            <option value="0.01">0.01</option>
            <option value="0.02">0.02</option>
            <option value="0.05">0.05</option>
          </select>
        </label>
        <label>Gap (track separation)
          <select name="gap" id="gap">
            <option value="250ms">250ms</option>
            <option value="500ms">500ms</option>
            <option value="1s">1s</option>
            <option value="2s">2s</option>
          </select>
        </label>
        <label>Settle
          <select name="settle" id="settle">
            <option value="500ms">500ms</option>
            <option value="1s">1s</option>
            <option value="2s">2s</option>
            <option value="3s">3s</option>
            <option value="6s">6s</option>
          </select>
        </label>
        <label>First clip
          <select name="clip" id="clip">
            <option value="3s">3s</option>
            <option value="5s">5s</option>
            <option value="8s">8s</option>
            <option value="10s">10s</option>
            <option value="12s">12s</option>
            <option value="15s">15s</option>
          </select>
        </label>
        <label>Miss wait
          <select name="miss_wait" id="miss_wait">
            <option value="10s">10s</option>
            <option value="15s">15s</option>
            <option value="20s">20s</option>
            <option value="30s">30s</option>
          </select>
        </label>
        <label>Full clip ring
          <select name="full_clip" id="full_clip">
            <option value="20s">20s</option>
            <option value="30s">30s</option>
            <option value="45s">45s</option>
            <option value="60s">60s</option>
          </select>
        </label>
        <label>Silence → idle
          <select name="silence" id="silence">
            <option value="10s">10s</option>
            <option value="15s">15s</option>
            <option value="20s">20s</option>
            <option value="30s">30s</option>
            <option value="60s">60s</option>
          </select>
        </label>
        <label>Recheck fallback
          <select name="recheck" id="recheck">
            <option value="30s">30s</option>
            <option value="1m">1m</option>
            <option value="2m">2m</option>
            <option value="3m">3m</option>
          </select>
        </label>
      </div>
    </fieldset>

    <fieldset>
      <legend>Idle / weather</legend>
      <div class="row">
        <label>Place
          <input type="text" name="place" id="place">
        </label>
        <label>Celsius
          <select name="celsius" id="celsius">
            <option value="false">No (°F)</option>
            <option value="true">Yes (°C)</option>
          </select>
        </label>
        <label>Latitude
          <input type="number" step="any" name="lat" id="lat">
        </label>
        <label>Longitude
          <input type="number" step="any" name="lon" id="lon">
        </label>
        <label>Weather refresh
          <select name="weather_every" id="weather_every">
            <option value="5m">5m</option>
            <option value="15m">15m</option>
            <option value="30m">30m</option>
            <option value="1h">1h</option>
          </select>
        </label>
        <label>Solar refresh <span>API uses ~15m samples; faster mostly burns credits</span>
          <select name="solar_every" id="solar_every">
            <option value="1m">1m</option>
            <option value="5m">5m</option>
            <option value="15m">15m</option>
            <option value="30m">30m</option>
            <option value="1h">1h</option>
          </select>
        </label>
        <label>Radar panel
          <select name="radar" id="radar">
            <option value="false">Off</option>
            <option value="true">On</option>
          </select>
        </label>
      </div>
    </fieldset>

    <fieldset>
      <legend>Screen</legend>
      <div class="row">
        <label>Size preset
          <select name="size_preset" id="size_preset">
            <option value="1448x1072">Paperwhite 7 landscape 1448×1072</option>
            <option value="1072x1448">Paperwhite 7 portrait 1072×1448</option>
            <option value="1024x758">Paperwhite 5/6 landscape 1024×758</option>
            <option value="758x1024">Paperwhite 5/6 portrait 758×1024</option>
          </select>
        </label>
        <label>Frontlight <span>Kindle flIntensity; applied by the device loop</span>
          <select name="frontlight" id="frontlight">
            <option value="-1">Leave alone</option>
            <option value="0">0 (off/min)</option>
            <option value="1">1</option>
            <option value="2">2</option>
            <option value="3">3</option>
            <option value="4">4</option>
            <option value="5">5</option>
            <option value="6">6</option>
            <option value="7">7</option>
            <option value="8">8</option>
            <option value="9">9</option>
            <option value="10">10</option>
            <option value="11">11</option>
            <option value="12">12</option>
            <option value="13">13</option>
            <option value="14">14</option>
            <option value="15">15</option>
            <option value="16">16</option>
            <option value="17">17</option>
            <option value="18">18</option>
            <option value="19">19</option>
            <option value="20">20</option>
            <option value="21">21</option>
            <option value="22">22</option>
            <option value="23">23</option>
            <option value="24">24 (max)</option>
          </select>
        </label>
      </div>
    </fieldset>

    <div class="actions">
      <button type="submit" id="save">Save</button>
      <button type="button" class="secondary" id="restart">Restart service</button>
      <button type="button" class="secondary" id="kindleStop">Restore Kindle UI</button>
      <div class="msg" id="msg"></div>
    </div>
  </form>
</main>
<script>
const fields = [
  "identify","threshold","settle","clip","miss_wait","full_clip","gap","silence","recheck",
  "weather_every","solar_every","celsius","radar","lat","lon","place"
];

function ensureOption(sel, value) {
  if (!sel || value === undefined || value === null || value === "") return;
  const v = String(value);
  if (![...sel.options].some(o => o.value === v)) {
    const opt = document.createElement("option");
    opt.value = v; opt.textContent = v + " (current)";
    sel.appendChild(opt);
  }
  sel.value = v;
}

function fill(data) {
  const s = data;
  ensureOption(identify, s.identify);
  ensureOption(threshold, s.threshold);
  for (const id of ["settle","clip","miss_wait","full_clip","gap","silence","recheck","weather_every","solar_every"]) {
    ensureOption(document.getElementById(id), s[id]);
  }
  celsius.value = String(!!s.celsius);
  radar.value = String(!!s.radar);
  place.value = s.place || "";
  lat.value = s.lat ?? "";
  lon.value = s.lon ?? "";
  const preset = (s.width||0) + "x" + (s.height||0);
  ensureOption(size_preset, preset);
  ensureOption(frontlight, (s.frontlight === undefined || s.frontlight === null) ? -1 : s.frontlight);
  tokenHint.textContent = s.has_audd_token ? "token set — leave blank to keep" : "required for audd/both";
}

async function load() {
  const res = await fetch("/settings.json", {cache:"no-store"});
  if (!res.ok) throw new Error(await res.text());
  fill(await res.json());
}

f.addEventListener("submit", async (e) => {
  e.preventDefault();
  msg.className = "msg";
  msg.textContent = "Saving…";
  save.disabled = true;
  try {
    const [w,h] = size_preset.value.split("x").map(Number);
    const body = {
      identify: identify.value,
      threshold: Number(threshold.value),
      settle: settle.value,
      clip: clip.value,
      miss_wait: miss_wait.value,
      full_clip: full_clip.value,
      gap: gap.value,
      silence: silence.value,
      recheck: recheck.value,
      weather_every: weather_every.value,
      solar_every: solar_every.value,
      celsius: celsius.value === "true",
      radar: radar.value === "true",
      lat: Number(lat.value),
      lon: Number(lon.value),
      place: place.value.trim(),
      width: w,
      height: h,
      frontlight: Number(frontlight.value),
    };
    const tok = audd_api_token.value.trim();
    if (tok) body.audd_api_token = tok;
    const res = await fetch("/settings.json", {
      method: "POST",
      headers: {"Content-Type":"application/json"},
      body: JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(text || res.statusText);
    const data = JSON.parse(text);
    fill({...data.settings, has_audd_token: data.has_audd_token});
    audd_api_token.value = "";
    msg.className = "msg ok";
    msg.textContent = "Saved.";
  } catch (err) {
    msg.className = "msg err";
    msg.textContent = String(err.message || err);
  } finally {
    save.disabled = false;
  }
});

kindleStop.addEventListener("click", async () => {
  if (!confirm("Tell the Kindle to restore Amazon UI and exit Now Playing?")) return;
  msg.className = "msg";
  msg.textContent = "Sending stop to Kindle…";
  try {
    const res = await fetch("/kindle/stop", {method:"POST"});
    if (!res.ok) throw new Error(await res.text() || res.statusText);
    msg.className = "msg ok";
    msg.textContent = "Stop requested — Kindle should restore UI within a few seconds (needs updated loop.sh). If not, hold power 20–40s.";
  } catch (err) {
    msg.className = "msg err";
    msg.textContent = String(err.message || err);
  }
});

restart.addEventListener("click", async () => {
  if (!confirm("Restart kindle-shazam now?")) return;
  msg.className = "msg";
  msg.textContent = "Restarting…";
  save.disabled = true;
  restart.disabled = true;
  try {
    const res = await fetch("/settings/restart", {method:"POST"});
    if (!res.ok) throw new Error(await res.text() || res.statusText);
    msg.className = "msg ok";
    msg.textContent = "Service restarting — reloading in a few seconds…";
    let tries = 0;
    const poll = async () => {
      tries++;
      try {
        const r = await fetch("/settings.json", {cache:"no-store"});
        if (r.ok) {
          location.reload();
          return;
        }
      } catch (_) {}
      if (tries < 30) setTimeout(poll, 1000);
      else {
        msg.className = "msg err";
        msg.textContent = "Restarted, but page could not reconnect — refresh manually.";
        save.disabled = false;
        restart.disabled = false;
      }
    };
    setTimeout(poll, 1500);
  } catch (err) {
    msg.className = "msg err";
    msg.textContent = String(err.message || err);
    save.disabled = false;
    restart.disabled = false;
  }
});

load().catch(err => {
  msg.className = "msg err";
  msg.textContent = String(err.message || err);
});
</script>
</body>
</html>
`