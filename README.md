# KindleID

KindleID turns a jailbroken Kindle into an always-on now-playing display. A Raspberry Pi on the LAN listens, identifies the track, and the Kindle redraws with cover art, title, artist, and year. When nothing is playing, it shows weather (and optional solar) instead.

**Project write-up:** [s16n.com/projects/kindleid](https://www.s16n.com/projects/kindleid/)

## Repo layout

| Path | Role |
| --- | --- |
| `cmd/kindle-shazam` | Go server (listen → detect → identify → render → HTTP `:8080`) |
| `kindle/extensions/kindleshazam` | KUAL extension (polls `/version`, paints with FBInk) |
| `deploy/pi` | Cross-build + systemd deploy to a Raspberry Pi |

Copy `config.example.json` → `config.local.json` (gitignored), then from the repo root:

```powershell
powershell -File deploy/pi/deploy.ps1
```

More Pi host detail: [`deploy/pi/README.md`](deploy/pi/README.md).
