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
  button, a.btn { background: var(--accent); color: #fff; border: 0; border-radius: 8px; padding: 10px 16px; font: inherit; cursor: pointer; text-decoration: none; display: inline-block; }
  button.secondary, a.btn.secondary { background: transparent; color: var(--ink); border: 1px solid var(--line); }
  button:disabled, a.btn.disabled { opacity: 0.6; cursor: default; pointer-events: none; }
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
        <label>Radar panel
          <select name="radar" id="radar">
            <option value="false">Off</option>
            <option value="true">On</option>
          </select>
        </label>
      </div>
    </fieldset>

    <fieldset>
      <legend>SolarEdge</legend>
      <div class="row">
        <label>Source <span>Modbus is live but can upset some inverters — keep refresh ≥15m</span>
          <select name="solar_source" id="solar_source">
            <option value="api">Cloud API (OAuth)</option>
            <option value="modbus">Modbus TCP (local)</option>
          </select>
        </label>
        <label>Refresh <span>API: free-tier credits, no polls 9pm–6am. Modbus forced ≥15m.</span>
          <select name="solar_every" id="solar_every">
            <option value="15m">15m</option>
            <option value="30m">30m</option>
            <option value="1h">1h</option>
            <option value="2h">2h</option>
            <option value="4h">4h</option>
          </select>
        </label>
        <label>Modbus host:port
          <input type="text" name="solar_modbus" id="solar_modbus" placeholder="192.168.1.50:502" autocomplete="off">
        </label>
        <label>Modbus unit id
          <input type="number" name="solar_unit" id="solar_unit" min="1" max="255" step="1" value="1">
        </label>
        <label>Site ID <span>from SolarEdge monitoring URL / OAuth</span>
          <input type="text" name="se_site_id" id="se_site_id" placeholder="1292007" autocomplete="off">
        </label>
      </div>
      <p class="sub" id="seStatus" style="margin:8px 0 4px">SolarEdge status…</p>
      <p class="sub" id="seHint" style="margin:0 0 8px;font-size:0.85rem"></p>
      <p class="sub" id="seRedirect" style="margin:0 0 8px;word-break:break-all;font-size:0.8rem"></p>
      <div class="actions" style="margin-top:0">
        <a class="btn secondary" id="seConnect" href="#" target="_blank" rel="noopener noreferrer">1. Connect SolarEdge (new tab)</a>
        <button type="button" class="secondary" id="seSaveSite">Save site ID</button>
      </div>
      <p class="sub" style="margin:8px 0;font-size:0.85rem"><strong>“127.0.0.1 refused to connect” is expected.</strong> Do not close that tab — copy the full address-bar URL (it contains <code>code=</code>) and paste it below.</p>
      <label style="margin-top:4px">2. Paste callback URL or code
        <input type="text" name="se_code" id="se_code" placeholder="http://127.0.0.1:8765/callback?code=…" autocomplete="off">
      </label>
      <div class="actions">
        <button type="button" class="secondary" id="seSubmitCode">3. Submit code</button>
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
  ensureOption(solar_source, s.solar_source || "api");
  solar_modbus.value = s.solar_modbus || "";
  solar_unit.value = s.solar_unit > 0 ? s.solar_unit : 1;
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

async function loadSolar() {
  try {
    const res = await fetch("/settings/solaredge.json", {cache:"no-store"});
    if (!res.ok) throw new Error(await res.text());
    const se = await res.json();
    if (se.site_id) se_site_id.value = se.site_id;
    let line = "API: ";
    if (!se.configured) line += "client id/secret missing on server";
    else if (se.authorized) line += "authorized" + (se.site_id ? " (site " + se.site_id + ")" : "");
    else line += "not authorized — Connect below";
    seStatus.textContent = line;
    seHint.textContent = se.authorize_hint || "";
    seRedirect.textContent = se.redirect_uri
      ? "Developer portal redirect URI (exact): " + se.redirect_uri
      : "";
    if (se.configured) {
      const ar = await fetch("/settings/solaredge/authorize", {cache:"no-store"});
      if (ar.ok) {
        const a = await ar.json();
        if (a.authorize_url) {
          seConnect.href = a.authorize_url;
          seConnect.classList.remove("disabled");
        }
      }
    } else {
      seConnect.href = "#";
      seConnect.classList.add("disabled");
    }
  } catch (err) {
    seStatus.textContent = "API status unavailable: " + (err.message || err);
  }
}

async function load() {
  const res = await fetch("/settings.json", {cache:"no-store"});
  if (!res.ok) throw new Error(await res.text());
  fill(await res.json());
  await loadSolar();
  const q = new URLSearchParams(location.search);
  if (q.get("solaredge") === "ok") {
    msg.className = "msg ok";
    msg.textContent = "SolarEdge authorized.";
    history.replaceState({}, "", "/settings");
  } else if (q.get("solaredge") === "err") {
    msg.className = "msg err";
    msg.textContent = q.get("msg") || "SolarEdge auth failed";
    history.replaceState({}, "", "/settings");
  }
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
      solar_source: solar_source.value,
      solar_modbus: solar_modbus.value.trim(),
      solar_unit: Number(solar_unit.value) || 1,
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

seConnect.addEventListener("click", (e) => {
  if (!seConnect.href || seConnect.href.endsWith("#") || seConnect.classList.contains("disabled")) {
    e.preventDefault();
    msg.className = "msg err";
    msg.textContent = "SolarEdge client id/secret not configured on the server.";
    return;
  }
  // Fresh authorize URL each click (state/nonce).
  e.preventDefault();
  (async () => {
    try {
      const ar = await fetch("/settings/solaredge/authorize", {cache:"no-store"});
      if (!ar.ok) throw new Error(await ar.text() || ar.statusText);
      const a = await ar.json();
      if (!a.authorize_url) throw new Error("no authorize_url");
      seConnect.href = a.authorize_url;
      const w = window.open(a.authorize_url, "_blank", "noopener,noreferrer");
      msg.className = "msg";
      if (!w) {
        msg.textContent = "Popup blocked — use the Connect link (right-click → Open in new tab), or click Connect again after allowing popups.";
        // Fall back: navigate via the anchor without prevent on next click.
        seConnect.removeAttribute("data-needs-gesture");
      } else {
        msg.textContent = "Approve in the new tab. Connection refused on 127.0.0.1 is normal — copy that tab’s full URL and paste below.";
      }
    } catch (err) {
      msg.className = "msg err";
      msg.textContent = String(err.message || err);
    }
  })();
});

seSaveSite.addEventListener("click", async () => {
  msg.className = "msg";
  msg.textContent = "Saving site ID…";
  try {
    const res = await fetch("/settings/solaredge.json", {
      method: "POST",
      headers: {"Content-Type":"application/json"},
      body: JSON.stringify({site_id: se_site_id.value.trim()}),
    });
    if (!res.ok) throw new Error(await res.text() || res.statusText);
    await loadSolar();
    msg.className = "msg ok";
    msg.textContent = "Site ID saved.";
  } catch (err) {
    msg.className = "msg err";
    msg.textContent = String(err.message || err);
  }
});

seSubmitCode.addEventListener("click", async () => {
  msg.className = "msg";
  msg.textContent = "Exchanging code…";
  try {
    const res = await fetch("/settings/solaredge.json", {
      method: "POST",
      headers: {"Content-Type":"application/json"},
      body: JSON.stringify({
        code: se_code.value.trim(),
        site_id: se_site_id.value.trim(),
      }),
    });
    if (!res.ok) throw new Error(await res.text() || res.statusText);
    se_code.value = "";
    await loadSolar();
    msg.className = "msg ok";
    msg.textContent = "SolarEdge authorized.";
  } catch (err) {
    msg.className = "msg err";
    msg.textContent = String(err.message || err);
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