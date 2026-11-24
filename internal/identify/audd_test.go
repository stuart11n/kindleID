package identify

import "testing"

func TestLastSeconds(t *testing.T) {
	in := make([]float32, 16000*20)
	for i := range in {
		in[i] = float32(i)
	}
	out := lastSeconds(in, 16000, 12)
	if len(out) != 16000*12 {
		t.Fatalf("len %d", len(out))
	}
	if out[0] != float32(16000*8) {
		t.Fatalf("start %v", out[0])
	}
}

func TestYearFromDate(t *testing.T) {
	if yearFromDate("1991-03-01") != "1991" {
		t.Fatal(yearFromDate("1991-03-01"))
	}
}
