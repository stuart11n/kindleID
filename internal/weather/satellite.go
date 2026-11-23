package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"sync"

	xdraw "golang.org/x/image/draw"
)

const (
	radarRadiusMiles = 100
	radarImageSize   = 900
	radarTileSize    = 256
	radarMaxZoom     = 7 // RainViewer radar tiles: max zoom 7
	milesPerDegLat   = 69.172
)

// FetchSatellite builds a radar map centered on the configured lat/lon
// covering about radarRadiusMiles in each direction.
func (c *Client) FetchSatellite(ctx context.Context) (image.Image, error) {
	if err := c.Resolve(ctx); err != nil {
		return nil, err
	}
	lat, lon := c.opts.Lat, c.opts.Lon
	if lat == 0 && lon == 0 {
		return nil, fmt.Errorf("radar: missing lat/lon")
	}

	host, path, err := c.rainViewerLatest(ctx)
	if err != nil {
		return nil, err
	}

	cosLat := math.Cos(lat * math.Pi / 180)
	if cosLat < 0.2 {
		cosLat = 0.2
	}
	dLat := radarRadiusMiles / milesPerDegLat
	dLon := radarRadiusMiles / (milesPerDegLat * cosLat)
	north, south := lat+dLat, lat-dLat
	east, west := lon+dLon, lon-dLon

	z := zoomForBounds(west, south, east, north, radarImageSize)
	if z > radarMaxZoom {
		z = radarMaxZoom
	}

	x0, y0 := latLonToWorldPixels(north, west, z)
	x1, y1 := latLonToWorldPixels(south, east, z)
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}

	tx0 := int(math.Floor(x0 / radarTileSize))
	ty0 := int(math.Floor(y0 / radarTileSize))
	tx1 := int(math.Floor((x1 - 1) / radarTileSize))
	ty1 := int(math.Floor((y1 - 1) / radarTileSize))
	n := 1 << z
	if tx0 < 0 {
		tx0 = 0
	}
	if ty0 < 0 {
		ty0 = 0
	}
	if tx1 >= n {
		tx1 = n - 1
	}
	if ty1 >= n {
		ty1 = n - 1
	}

	// Esri light-gray canvas (z/y/x) — no API key. OSM also works as fallback.
	radarURL := fmt.Sprintf("%s%s/%d/%d", host, path, radarTileSize, z)

	tilesX := tx1 - tx0 + 1
	tilesY := ty1 - ty0 + 1
	stitched := image.NewRGBA(image.Rect(0, 0, tilesX*radarTileSize, tilesY*radarTileSize))

	type job struct{ x, y int }
	jobs := make([]job, 0, tilesX*tilesY)
	for y := ty0; y <= ty1; y++ {
		for x := tx0; x <= tx1; x++ {
			jobs = append(jobs, job{x, y})
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			bx := (j.x - tx0) * radarTileSize
			by := (j.y - ty0) * radarTileSize
			dst := image.Rect(bx, by, bx+radarTileSize, by+radarTileSize)

			base, err := c.fetchTile(ctx, fmt.Sprintf(
				"https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Light_Gray_Base/MapServer/tile/%d/%d/%d",
				z, j.y, j.x))
			if err != nil {
				base, err = c.fetchTile(ctx, fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", z, j.x, j.y))
			}
			mu.Lock()
			if err == nil {
				xdraw.Draw(stitched, dst, base, base.Bounds().Min, xdraw.Src)
			} else {
				xdraw.Draw(stitched, dst, &image.Uniform{C: color.White}, image.Point{}, xdraw.Src)
			}
			mu.Unlock()
			if rad, err := c.fetchTile(ctx, fmt.Sprintf("%s/%d/%d/2/1_1.png", radarURL, j.x, j.y)); err == nil {
				mu.Lock()
				xdraw.Draw(stitched, dst, rad, rad.Bounds().Min, xdraw.Over)
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()

	crop := image.Rect(
		int(math.Round(x0))-tx0*radarTileSize,
		int(math.Round(y0))-ty0*radarTileSize,
		int(math.Round(x1))-tx0*radarTileSize,
		int(math.Round(y1))-ty0*radarTileSize,
	)
	crop = crop.Intersect(stitched.Bounds())
	if crop.Empty() {
		return nil, fmt.Errorf("radar: empty crop")
	}
	out := image.NewRGBA(image.Rect(0, 0, radarImageSize, radarImageSize))
	xdraw.CatmullRom.Scale(out, out.Bounds(), stitched, crop, xdraw.Src, nil)
	return out, nil
}

func (c *Client) rainViewerLatest(ctx context.Context) (host, path string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.rainviewer.com/public/weather-maps.json", nil)
	if err != nil {
		return "", "", err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("rainviewer: %s", res.Status)
	}
	var raw struct {
		Host  string `json:"host"`
		Radar struct {
			Past []struct {
				Path string `json:"path"`
			} `json:"past"`
		} `json:"radar"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return "", "", err
	}
	if raw.Host == "" || len(raw.Radar.Past) == 0 || raw.Radar.Past[len(raw.Radar.Past)-1].Path == "" {
		return "", "", fmt.Errorf("rainviewer: no radar frames")
	}
	return raw.Host, raw.Radar.Past[len(raw.Radar.Past)-1].Path, nil
}

func (c *Client) fetchTile(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kindle-shazam/1.0")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}
	img, _, err := image.Decode(res.Body)
	return img, err
}

func latLonToWorldPixels(lat, lon float64, z int) (x, y float64) {
	n := float64(int(1) << z)
	x = (lon + 180) / 360 * n * radarTileSize
	sinLat := math.Sin(lat * math.Pi / 180)
	if sinLat > 0.9999 {
		sinLat = 0.9999
	}
	if sinLat < -0.9999 {
		sinLat = -0.9999
	}
	y = (0.5 - math.Log((1+sinLat)/(1-sinLat))/(4*math.Pi)) * n * radarTileSize
	return x, y
}

func zoomForBounds(west, south, east, north float64, size int) int {
	best := 3
	for z := 3; z <= radarMaxZoom; z++ {
		x0, y0 := latLonToWorldPixels(north, west, z)
		x1, y1 := latLonToWorldPixels(south, east, z)
		w := x1 - x0
		h := y1 - y0
		if w <= float64(size)*1.35 && h <= float64(size)*1.35 {
			best = z
		}
	}
	return best
}
