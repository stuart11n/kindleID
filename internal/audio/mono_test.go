package audio

import "testing"

func TestStereoToMonoPicksLoudChannel(t *testing.T) {
	// Left quiet, right hot — one-cable-on-right case.
	in := make([]float32, 8)
	for i := 0; i < 4; i++ {
		in[i*2] = 0.01
		in[i*2+1] = 0.5
	}
	out := StereoToMono(in)
	if len(out) != 4 {
		t.Fatalf("len %d", len(out))
	}
	for _, v := range out {
		if v != 0.5 {
			t.Fatalf("wanted R=0.5, got %v (choice=%s)", v, ChannelChoice(in))
		}
	}
}

func TestStereoToMonoMixesBalanced(t *testing.T) {
	in := []float32{0.4, 0.4, -0.4, -0.4}
	out := StereoToMono(in)
	if ChannelChoice(in) != "mix" {
		t.Fatalf("choice %s", ChannelChoice(in))
	}
	if out[0] != 0.4 || out[1] != -0.4 {
		t.Fatalf("%v", out)
	}
}
