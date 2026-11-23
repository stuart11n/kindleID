package audio

import (
	"fmt"
	"strings"
)

// Device is a capture endpoint the server can open.
type Device struct {
	ID   string
	Name string
}

// Pick chooses a capture device whose name contains match.
// When several match, a name containing "microphone" wins.
// match "first" (or "*") selects the first listed capture device.
func Pick(devices []Device, match string) (Device, error) {
	match = strings.TrimSpace(match)
	if len(devices) == 0 {
		return Device{}, fmt.Errorf("no capture devices found")
	}
	if match == "" {
		return Device{}, fmt.Errorf("device name is empty")
	}
	if strings.EqualFold(match, "first") || match == "*" {
		return devices[0], nil
	}
	needle := strings.ToLower(match)
	var hits []Device
	for _, d := range devices {
		if strings.Contains(strings.ToLower(d.Name), needle) || strings.EqualFold(d.ID, match) {
			hits = append(hits, d)
		}
	}
	if len(hits) == 0 {
		return Device{}, fmt.Errorf("no capture device matches %q", match)
	}
	for _, d := range hits {
		if strings.Contains(strings.ToLower(d.Name), "microphone") {
			return d, nil
		}
	}
	return hits[0], nil
}
