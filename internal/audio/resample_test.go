package audio

import "testing"

func TestDecimate3(t *testing.T) {
	in := []float32{1, 2, 3, 4, 5, 6}
	out := Decimate3(in)
	if len(out) != 2 || out[0] != 2 || out[1] != 5 {
		t.Fatalf("%v", out)
	}
}

func TestResamplerRatio(t *testing.T) {
	in := make([]float32, 48000)
	for i := range in {
		in[i] = 0.25
	}
	out := NewResampler(48000, 16000).Process(in)
	if len(out) < 15900 || len(out) > 16000 {
		t.Fatalf("got %d samples, want about 16000", len(out))
	}
	if out[100] < 0.2 || out[100] > 0.3 {
		t.Fatalf("level = %f", out[100])
	}
}

func TestMixdown(t *testing.T) {
	got := Mixdown([]float32{1, 3, 1, 3}, 2)
	if len(got) != 2 || got[0] != 2 || got[1] != 2 {
		t.Fatalf("got %v", got)
	}
}
