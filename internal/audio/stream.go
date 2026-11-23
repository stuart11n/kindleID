package audio

// Stream is a mono 16 kHz capture.
type Stream interface {
	Name() string
	SampleRate() int
	Chunks() <-chan []float32
	Close() error
}
