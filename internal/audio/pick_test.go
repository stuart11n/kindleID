package audio

import "testing"

func TestPickPrefersMicrophone(t *testing.T) {
	devices := []Device{
		{ID: "spk", Name: "Speakers (Yeti Nano)"},
		{ID: "mic", Name: "Microphone (Yeti Nano)"},
	}
	got, err := Pick(devices, "Yeti")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "mic" {
		t.Fatalf("got %s, want microphone", got.Name)
	}
}

func TestPickMiss(t *testing.T) {
	_, err := Pick([]Device{{ID: "a", Name: "Pimax"}}, "Yeti")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPickFirst(t *testing.T) {
	devices := []Device{
		{ID: "plughw:1,0", Name: "USB Audio"},
		{ID: "plughw:2,0", Name: "Other"},
	}
	got, err := Pick(devices, "first")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "plughw:1,0" {
		t.Fatalf("got %s", got.ID)
	}
}
