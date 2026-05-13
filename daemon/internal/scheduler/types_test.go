package scheduler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRunJSONShape(t *testing.T) {
	r := Run{
		ID:          "r1",
		Persona:     "marketing",
		Trigger:     TriggerTick,
		Status:      StatusQueued,
		Attempt:     1,
		ScheduledAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
		CreatedAt:   time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"retry_of":null`) {
		t.Errorf(`retry_of not rendered as null: %s`, got)
	}
	if !strings.Contains(got, `"failure_class":""`) {
		t.Errorf(`failure_class not rendered as empty string: %s`, got)
	}
}
