package audio

// StereoToMono picks a mono stream from interleaved stereo float samples.
// With only one cable plugged in, averaging with a dead channel kills level
// and fingerprints; use the louder side when it clearly dominates.
func StereoToMono(stereo []float32) []float32 {
	n := len(stereo) / 2
	if n == 0 {
		return nil
	}
	var sumL, sumR float64
	for i := 0; i < n; i++ {
		l := float64(stereo[i*2])
		r := float64(stereo[i*2+1])
		sumL += l * l
		sumR += r * r
	}
	useL := sumL >= sumR
	// If both are in the same ballpark, average (true stereo).
	mix := false
	if sumL > 0 && sumR > 0 {
		hi, lo := sumL, sumR
		if sumR > sumL {
			hi, lo = sumR, sumL
		}
		if hi < lo*4 {
			mix = true
		}
	}
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		l := stereo[i*2]
		r := stereo[i*2+1]
		if mix {
			out[i] = (l + r) * 0.5
		} else if useL {
			out[i] = l
		} else {
			out[i] = r
		}
	}
	return out
}

// ChannelChoice describes which side StereoToMono would prefer for this chunk.
func ChannelChoice(stereo []float32) string {
	n := len(stereo) / 2
	if n == 0 {
		return "none"
	}
	var sumL, sumR float64
	for i := 0; i < n; i++ {
		l := float64(stereo[i*2])
		r := float64(stereo[i*2+1])
		sumL += l * l
		sumR += r * r
	}
	if sumL > 0 && sumR > 0 {
		hi, lo := sumL, sumR
		if sumR > sumL {
			hi, lo = sumR, sumL
		}
		if hi < lo*4 {
			return "mix"
		}
	}
	if sumL >= sumR {
		return "L"
	}
	return "R"
}
