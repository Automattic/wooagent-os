package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTTLDays_Defaults(t *testing.T) {
	// Unset → 30 days for both windows.
	t.Setenv("WOOAGENT_DISMISS_TTL_DAYS", "")
	t.Setenv("WOOAGENT_RUNS_TTL_DAYS", "")
	if got := dismissTTLDays(&bytes.Buffer{}); got != 30 {
		t.Errorf("dismissTTLDays default = %d, want 30", got)
	}
	if got := runsTTLDays(&bytes.Buffer{}); got != 30 {
		t.Errorf("runsTTLDays default = %d, want 30", got)
	}
}

func TestRunsTTLDays_Override(t *testing.T) {
	t.Setenv("WOOAGENT_RUNS_TTL_DAYS", "7")
	if got := runsTTLDays(&bytes.Buffer{}); got != 7 {
		t.Errorf("runsTTLDays = %d, want 7", got)
	}
}

func TestRunsTTLDays_InvalidWarnsAndFallsBack(t *testing.T) {
	for _, raw := range []string{"abc", "-5", "3.5"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("WOOAGENT_RUNS_TTL_DAYS", raw)
			var buf bytes.Buffer
			got := runsTTLDays(&buf)
			if got != 30 {
				t.Errorf("runsTTLDays(%q) = %d, want 30", raw, got)
			}
			if !strings.Contains(buf.String(), "ignoring invalid WOOAGENT_RUNS_TTL_DAYS") {
				t.Errorf("expected a warning for %q, got: %q", raw, buf.String())
			}
		})
	}
}

// ttlDuration has to special-case zero. The Sweeper reads a zero duration as
// "field unset, use my default", so passing 0 straight through would silently
// mean 30 days — the opposite of the documented "0 = purge now" escape hatch.
func TestTTLDuration(t *testing.T) {
	for _, tc := range []struct {
		days int
		want time.Duration
	}{
		{30, 30 * 24 * time.Hour},
		{1, 24 * time.Hour},
		{0, time.Nanosecond},
	} {
		if got := ttlDuration(tc.days); got != tc.want {
			t.Errorf("ttlDuration(%d) = %v, want %v", tc.days, got, tc.want)
		}
	}
}

// Guards the actual bug this replaced: a zero day count must not round-trip
// into the Sweeper's default window.
func TestTTLDuration_ZeroIsNotThirtyDays(t *testing.T) {
	if got := ttlDuration(0); got >= 24*time.Hour {
		t.Errorf("ttlDuration(0) = %v; zero must mean immediate, not a full window", got)
	}
}
