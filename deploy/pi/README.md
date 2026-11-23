# Raspberry Pi host (Kindle Shazam)

Run the now-playing server on a Raspberry Pi on the LAN. The Kindle polls
`http://<pi-ip>:8080`.

Tested on an original **2011.12 Model B** (ARMv6) with **Raspberry Pi OS Legacy
Lite 32-bit (Bookworm)** and a USB audio capture device.

## Config (no secrets/personal data in git)

| File | Purpose |
| --- | --- |
| `config.example.json` | Placeholders committed to the repo |
| `config.local.json` | Your real values — **gitignored**; copy from the example |

```json
{
  "lat": 0,
  "lon": 0,
  "place": "YOUR_CITY",
  "solar_modbus": "SOLAREDGE_HOST:502",
  "solar_unit": 1,
  "pi_ssh": "USER@PI_HOST",
  "kindle_server_url": "http://PI_HOST:8080"
}
```

Site defaults are baked into the Pi binary at **deploy** time via `-ldflags`
(from `config.local.json`). The Kindle `server.url` in git is the placeholder
`http://PI_HOST:8080`; deploy writes the real URL onto a mounted Kindle.

## OS image

Use **Legacy Lite armhf** (Bookworm) for Pi 1 / Zero / CM1:

- https://downloads.raspberrypi.com/raspios_oldstable_lite_armhf/

Enable SSH and create a user (Imager OS customisation).

## Deploy (Windows → Pi + Kindle)

From the repo root, with `config.local.json` filled in:

```powershell
powershell -File deploy/pi/deploy.ps1
```

That script:

1. Cross-compiles `linux/arm` `GOARM=6` with ldflags from `config.local.json`
2. `scp`s the binary + unit to `pi_ssh`, installs, restarts `kindle-shazam`
3. If a Kindle volume is mounted, sets `extensions/kindleshazam/server.url`
   to `kindle_server_url`

First-time only on the Pi: `sudo apt-get install -y alsa-utils`, and optionally
`sudo bash deploy/pi/harden-sd.sh` (see below).

Manual build (placeholders only — prefer deploy.ps1):

```bash
GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 \
  go build -o kindle-shazam-linux-armv6 ./cmd/kindle-shazam
```

## Cutting SD-card writes

```bash
sudo bash deploy/pi/harden-sd.sh
```

1. **Disables swap** (`dphys-swapfile` → 0, removes `/var/swap`)
2. **Masks** `apt-daily`, `apt-daily-upgrade`, `man-db`, `dpkg-db-backup` timers
3. **Adds `commit=600`** to root ext4 (keeps `noatime`) and remounts

Also: app `/log` is RAM-only (50 lines), unit uses `StandardOutput=null` /
`StandardError=null`, journal should stay `Storage=volatile`.

## Ops notes

- Weather uses a long HTTP timeout and startup retries (Pi 1 TLS is slow).
- After install, `ls -l /usr/local/bin/kindle-shazam` must be non-zero
  (`203/EXEC` = empty binary).
- Find the Pi by MAC OUI `b8:27:eb` / `dc:a6:32` if DHCP moves the address.
