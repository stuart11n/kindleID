package detect

import (
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		SampleRate: 1000,
		Threshold:  0.02,
		Settle:     6 * time.Second,
		Clip:       10 * time.Second,
		Gap:        2 * time.Second,
		Silence:    20 * time.Second,
		Recheck:    time.Minute,
	}
}

func tone(n int, amp float32) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = amp
	}
	return s
}

func TestThumpDoesNotIdentify(t *testing.T) {
	m := New(testConfig())
	if act := m.Push(tone(1000, 0.2)); act.Identify || act.ShowNothing {
		t.Fatalf("thump produced %+v", act)
	}
	if act := m.Push(tone(3000, 0)); act.Identify {
		t.Fatal("silence after a thump identified")
	}
}

func TestTrackStartSendsTenSeconds(t *testing.T) {
	m := New(testConfig())
	if act := m.Push(tone(9000, 0.2)); act.Identify {
		t.Fatal("identified before 10s")
	}
	act := m.Push(tone(1000, 0.2))
	if !act.Identify || act.Reason != "start" || len(act.Clip) != 10000 {
		t.Fatalf("start action = %+v len=%d", act, len(act.Clip))
	}
	if !m.Finish(act.Generation, true) {
		t.Fatal("result rejected")
	}
	if m.State() != "playing" {
		t.Fatalf("state %s", m.State())
	}
}

func TestShortGapDoesNotResetSettle(t *testing.T) {
	m := New(testConfig())
	m.Push(tone(4000, 0.2))
	m.Push(tone(1000, 0))
	act := m.Push(tone(5000, 0.2))
	if !act.Identify || act.Reason != "start" {
		t.Fatalf("1s dip reset the track: %+v", act)
	}
}

func TestGapBetweenTracksRearms(t *testing.T) {
	m := New(testConfig())
	act := m.Push(tone(10000, 0.2))
	if !act.Identify {
		t.Fatal("missing first identify")
	}
	m.Finish(act.Generation, true)
	m.Push(tone(2000, 0))
	if act := m.Push(tone(10000, 0.2)); !act.Identify || act.Reason != "start" {
		t.Fatalf("gap did not start a new identify: %+v", act)
	}
}

func TestRecheckOnceAMinute(t *testing.T) {
	m := New(testConfig())
	act := m.Push(tone(10000, 0.2))
	m.Finish(act.Generation, true)
	if act := m.Push(tone(59000, 0.2)); act.Identify {
		t.Fatal("rechecked early")
	}
	act = m.Push(tone(1000, 0.2))
	if !act.Identify || act.Reason != "recheck" {
		t.Fatalf("recheck = %+v", act)
	}
}

func TestSilenceShowsNothingOnce(t *testing.T) {
	m := New(testConfig())
	act := m.Push(tone(10000, 0.2))
	m.Finish(act.Generation, true)
	act = m.Push(tone(20000, 0))
	if !act.ShowNothing {
		t.Fatal("expected nothing playing")
	}
	if late := m.Finish(act.Generation, false); late {
		t.Fatal("stale generation was accepted")
	}
	// The nothing event bumps the generation, so the in-flight id is stale.
	if again := m.Push(tone(5000, 0)); again.ShowNothing {
		t.Fatal("nothing playing repeated")
	}
}

func TestFailedIdentifyDoesNotClearScreen(t *testing.T) {
	m := New(testConfig())
	act := m.Push(tone(10000, 0.2))
	if !m.Finish(act.Generation, false) {
		t.Fatal("miss was rejected")
	}
	if m.State() != "playing" {
		t.Fatalf("state %s, want playing so the next minute retries", m.State())
	}
}

func TestMissRecheckBackoff(t *testing.T) {
	m := New(testConfig())
	if wait := m.ScheduleMissRecheck(); wait != 10*time.Second {
		t.Fatalf("first miss wait %s, want 10s", wait)
	}
	if wait := m.ScheduleMissRecheck(); wait != 15*time.Second {
		t.Fatalf("second miss wait %s, want 15s", wait)
	}
	for i := 0; i < 6; i++ {
		if wait := m.ScheduleMissRecheck(); wait != 15*time.Second {
			t.Fatalf("miss %d wait %s, want 15s", i+3, wait)
		}
	}
	// missFails is now 8; next bumps to 9 → 30s
	if wait := m.ScheduleMissRecheck(); wait != 30*time.Second {
		t.Fatalf("later miss wait %s, want 30s", wait)
	}
	m.ScheduleRecheck(2 * time.Minute)
	if wait := m.ScheduleMissRecheck(); wait != 10*time.Second {
		t.Fatalf("after match wait %s, want 10s", wait)
	}
}
