package identify

import "testing"

func TestStatsFooterHidden(t *testing.T) {
	st := newStats()
	st.RecordAttempt("shazam", false)
	st.RecordAttempt("audd", true)
	if got := st.Footer(); got != "" {
		t.Fatalf("footer should be hidden, got %q", got)
	}
}
