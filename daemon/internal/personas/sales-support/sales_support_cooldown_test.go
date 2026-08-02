package salessupport

import (
	"testing"
	"time"
)

func TestCooldown_IncludesSkippedWindow(t *testing.T) {
	c := SalesSupport{}.Cooldown()
	if c.TargetKey != "order_id" {
		t.Errorf("TargetKey: got %q want order_id", c.TargetKey)
	}
	if c.Skipped != 30*24*time.Hour {
		t.Errorf("Skipped: got %v want 720h", c.Skipped)
	}
}
