package audio

// Resampler linearly converts a mono stream to a new sample rate.
type Resampler struct {
	step float64
	pos  float64
	buf  []float32
}

func NewResampler(from, to int) *Resampler {
	if from <= 0 || to <= 0 {
		from, to = 1, 1
	}
	return &Resampler{step: float64(from) / float64(to)}
}

// Process appends input and returns the newly produced output samples.
func (r *Resampler) Process(in []float32) []float32 {
	if len(in) == 0 {
		return nil
	}
	if r.step == 1 {
		out := make([]float32, len(in))
		copy(out, in)
		return out
	}
	r.buf = append(r.buf, in...)
	var out []float32
	for r.pos+1 < float64(len(r.buf)) {
		i := int(r.pos)
		frac := float32(r.pos - float64(i))
		s := r.buf[i]*(1-frac) + r.buf[i+1]*frac
		out = append(out, s)
		r.pos += r.step
	}
	drop := int(r.pos)
	if drop > 0 {
		r.buf = append([]float32(nil), r.buf[drop:]...)
		r.pos -= float64(drop)
	}
	return out
}

// Mixdown averages interleaved channels into mono.
func Mixdown(interleaved []float32, channels int) []float32 {
	if channels <= 1 {
		return interleaved
	}
	frames := len(interleaved) / channels
	out := make([]float32, frames)
	for i := 0; i < frames; i++ {
		var sum float32
		for c := 0; c < channels; c++ {
			sum += interleaved[i*channels+c]
		}
		out[i] = sum / float32(channels)
	}
	return out
}
