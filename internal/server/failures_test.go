package server

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncodeWAVHeader(t *testing.T) {
	samples := make([]float32, 16000)
	for i := range samples {
		samples[i] = 0.25
	}
	wav := encodeWAV(samples, 16000)
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("bad riff")
	}
	if binary.LittleEndian.Uint16(wav[22:]) != 1 {
		t.Fatal("not mono")
	}
	if binary.LittleEndian.Uint32(wav[24:]) != 16000 {
		t.Fatal("bad rate")
	}
	if len(wav) != 44+16000*2 {
		t.Fatalf("len %d", len(wav))
	}
}

func TestFailureStoreKeepsTen(t *testing.T) {
	s := NewFailureStore()
	for i := 0; i < 12; i++ {
		s.Add([]float32{0.1, -0.1, 0.2}, 16000, "recheck")
	}
	list := s.list()
	if len(list) != 10 {
		t.Fatalf("got %d", len(list))
	}
	if list[0].ID != 12 || list[9].ID != 3 {
		t.Fatalf("order %+v %+v", list[0], list[9])
	}
}

func TestMatchFailuresHTTP(t *testing.T) {
	d := NewDisplay("test", 0.01)
	d.Failures().Add([]float32{0.5, -0.5, 0.25, -0.25}, 16000, "start")
	h := d.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/match-failures?format=json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if !contains(rr.Body.String(), `"count": 1`) {
		t.Fatalf("body %s", rr.Body.String())
	}

	list := d.Failures().list()
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", list[0].URL, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("wav status %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "audio/wav" {
		t.Fatalf("ct %s", ct)
	}
	if string(rr.Body.Bytes()[0:4]) != "RIFF" {
		t.Fatal("not wav")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
