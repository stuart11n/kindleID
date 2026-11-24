package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	base := DefaultSettingsValues()
	base.Identify = "both"
	base.AudDToken = "secret"
	s, err := NewSettings(path, base)
	if err != nil {
		t.Fatal(err)
	}
	v := s.GetRaw()
	v.Identify = "shazam"
	v.Gap = "250ms"
	v.AudDToken = "" // keep existing
	if err := s.Save(v); err != nil {
		t.Fatal(err)
	}
	s2, err := NewSettings(path, DefaultSettingsValues())
	if err != nil {
		t.Fatal(err)
	}
	got := s2.GetRaw()
	if got.Identify != "shazam" || got.Gap != "250ms" {
		t.Fatalf("got %+v", got)
	}
	if got.AudDToken != "secret" {
		t.Fatalf("token not kept: %q", got.AudDToken)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
