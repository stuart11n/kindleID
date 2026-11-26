#!/usr/bin/env python3
"""Crop labeled icons from the Met-style sheet and convert for Kindle e-ink."""
from __future__ import annotations

import json
import os

import numpy as np
from PIL import Image

SHEET = "assets/weather/sheet.png"
OUT = "internal/render/weathericons"

LABELS = [
    "sunny",
    "clear_night",
    "white_cloud",
    "sunny_intervals",
    "partly_cloudy_night",
    "grey_cloud",
    "drizzle",
    "light_rain",
    "light_rain_shower",
    "light_rain_shower_night",
    "heavy_rain",
    "heavy_rain_shower",
    "heavy_rain_shower_night",
    "thunder_storm",
    "thundery_shower",
    "thundery_shower_night",
    "sleet",
    "sleet_shower",
    "sleet_shower_night",
    "hail",
    "hail_shower",
    "hail_shower_night",
    "light_snow",
    "light_snow_shower",
    "light_snow_shower_night",
    "heavy_snow",
    "heavy_snow_shower",
    "heavy_snow_shower_night",
    "tropical_storm",
    "fog",
    "hazy",
    "mist",
    "sandstorm",
    "fog_box",
    "ice",
    "rain_drops",
    "snowflake",
    "wind",
    "wind_gust_a",
    "wind_gust_b",
    "flood",
    "coast_sea",
]


def to_eink(icon_rgba: Image.Image) -> Image.Image:
    arr = np.array(icon_rgba).astype(np.float32)
    rgb, a = arr[:, :, :3], arr[:, :, 3]
    mx = rgb.max(axis=2)
    mn = rgb.min(axis=2)
    sat = (mx - mn) / (mx + 1e-6)
    lum = 0.299 * rgb[:, :, 0] + 0.587 * rgb[:, :, 1] + 0.114 * rgb[:, :, 2]
    outg = np.full(lum.shape, 255.0, dtype=np.float32)
    opaque = a > 40
    light = opaque & (lum > 180) & (lum <= 245) & (sat < 0.15)
    outg[light] = 210
    dark = opaque & (lum <= 180) & (sat < 0.2)
    outg[dark] = np.clip(lum[dark] * 0.35, 0, 80)
    color = opaque & (sat >= 0.15)
    outg[color] = 25
    alpha = a / 255.0
    outg = outg * alpha + 255 * (1 - alpha)
    g = Image.fromarray(np.clip(outg, 0, 255).astype(np.uint8), mode="L")
    mask = np.array(g) < 250
    if mask.any():
        ys, xs = np.where(mask)
        pad = 4
        g = g.crop(
            (
                max(0, int(xs.min()) - pad),
                max(0, int(ys.min()) - pad),
                min(g.size[0], int(xs.max()) + 1 + pad),
                min(g.size[1], int(ys.max()) + 1 + pad),
            )
        )
    return g


def main() -> None:
    im = Image.open(SHEET).convert("RGBA")
    cols, rows = 6, 7
    x0, y0, x1, y1 = 30, 60, 1470, 2140
    cw = (x1 - x0) / cols
    ch = (y1 - y0) / rows
    os.makedirs(OUT, exist_ok=True)
    manifest = {}
    for r in range(rows):
        for c in range(cols):
            i = r * cols + c
            name = LABELS[i]
            xa = int(x0 + c * cw)
            ya = int(y0 + r * ch)
            xb = int(x0 + (c + 1) * cw)
            yb = int(y0 + (r + 1) * ch)
            cell = im.crop((xa, ya, xb, yb))
            ih = int(cell.size[1] * 0.58)
            icon = cell.crop((8, 8, cell.size[0] - 8, ih))
            g = to_eink(icon)
            side = max(g.size) + 8
            canvas = Image.new("L", (side, side), 255)
            canvas.paste(g, ((side - g.size[0]) // 2, (side - g.size[1]) // 2))
            path = os.path.join(OUT, f"{name}.png")
            canvas.save(path, optimize=True)
            manifest[name] = {"row": r, "col": c, "w": canvas.size[0], "h": canvas.size[1]}
            print(name, canvas.size)
    with open(os.path.join(OUT, "manifest.json"), "w", encoding="utf-8") as f:
        json.dump(manifest, f, indent=2)
    print("done", len(manifest))


if __name__ == "__main__":
    main()
