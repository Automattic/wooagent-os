package pep

import (
	"strings"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// at returns a fixed time at HH:MM:00 in UTC for a synthetic 2026-05-20
// day. Tests use it to anchor "now" inside / outside the configured
// window. UTC chosen to keep tests deterministic — the policy itself
// reads daemon-local TZ which in CI is UTC anyway.
func at(hour, minute int) time.Time {
	return time.Date(2026, 5, 20, hour, minute, 0, 0, time.UTC)
}

func TestOperatorHoursPolicy_InsideWindowAllow(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(10, 30),
		Agent: AgentSettings{ApplyHoursStart: "09:00", ApplyHoursEnd: "17:00"},
	}
	deny, _ := pol.Evaluate(req, ctx)
	if deny {
		t.Error("inside 09:00-17:00 at 10:30 should allow")
	}
}

func TestOperatorHoursPolicy_OutsideWindowDenies(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(20, 30),
		Agent: AgentSettings{ApplyHoursStart: "09:00", ApplyHoursEnd: "17:00"},
	}
	deny, reason := pol.Evaluate(req, ctx)
	if !deny {
		t.Fatal("outside 09:00-17:00 at 20:30 should deny")
	}
	if !strings.Contains(reason, "09:00") || !strings.Contains(reason, "17:00") {
		t.Errorf("reason should include the configured window, got %q", reason)
	}
}

// Overnight: window 22:00-06:00. "Inside" means now >= 22:00 OR now < 06:00.
func TestOperatorHoursPolicy_OvernightWindowAtNightAllow(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(2, 0),
		Agent: AgentSettings{ApplyHoursStart: "22:00", ApplyHoursEnd: "06:00"},
	}
	deny, _ := pol.Evaluate(req, ctx)
	if deny {
		t.Error("overnight 22:00-06:00 at 02:00 should allow (inside)")
	}
}

func TestOperatorHoursPolicy_OvernightWindowAtNoonDenies(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(12, 0),
		Agent: AgentSettings{ApplyHoursStart: "22:00", ApplyHoursEnd: "06:00"},
	}
	deny, _ := pol.Evaluate(req, ctx)
	if !deny {
		t.Error("overnight 22:00-06:00 at 12:00 should deny (outside)")
	}
}

func TestOperatorHoursPolicy_BothNullAllow(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{Now: at(3, 0)} // 3am with no window = no-op
	deny, _ := pol.Evaluate(req, ctx)
	if deny {
		t.Error("unset hours should allow")
	}
}

func TestOperatorHoursPolicy_MalformedHoursDenies(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(10, 0),
		Agent: AgentSettings{ApplyHoursStart: "not-a-time", ApplyHoursEnd: "17:00"},
	}
	deny, reason := pol.Evaluate(req, ctx)
	if !deny {
		t.Fatal("malformed hours should fail-closed")
	}
	if !strings.Contains(reason, "invalid") {
		t.Errorf("reason should mention invalid config, got %q", reason)
	}
}

func TestOperatorHoursPolicy_OperatorSourcedAllow(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceOperator, Persona: manifest.PersonaMarketing}
	ctx := EvalContext{
		Now:   at(20, 30),
		Agent: AgentSettings{ApplyHoursStart: "09:00", ApplyHoursEnd: "17:00"},
	}
	deny, _ := pol.Evaluate(req, ctx)
	if deny {
		t.Error("operator-sourced should allow regardless of hours")
	}
}

func TestOperatorHoursPolicy_NonApplyAllow(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentRead, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(20, 30),
		Agent: AgentSettings{ApplyHoursStart: "09:00", ApplyHoursEnd: "17:00"},
	}
	deny, _ := pol.Evaluate(req, ctx)
	if deny {
		t.Error("non-apply intent should allow")
	}
}

// XOR config (only start set, or only end) is treated as malformed —
// PATCH handler rejects it but defense-in-depth here too.
func TestOperatorHoursPolicy_XORDenies(t *testing.T) {
	pol := &operatorHoursPolicy{}
	req := Request{Intent: IntentApply, Source: SourceAgent}
	ctx := EvalContext{
		Now:   at(10, 0),
		Agent: AgentSettings{ApplyHoursStart: "09:00", ApplyHoursEnd: ""},
	}
	deny, _ := pol.Evaluate(req, ctx)
	if !deny {
		t.Error("XOR hours config should fail-closed")
	}
}
