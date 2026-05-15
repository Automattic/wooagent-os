package pep

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

func TestCheckBudget_UnderBudgetAllows(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed, got reason %q", dec.Reason)
	}
	today := time.Now().Local().Format("2006-01-02")
	var calls int
	if err := db.QueryRow(
		`SELECT call_count FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	).Scan(&calls); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if calls != 1 {
		t.Errorf("call_count = %d, want 1", calls)
	}
}

func TestCheckBudget_OverCostDenies(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	today := time.Now().Local().Format("2006-01-02")
	if _, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		"marketing", today, 10.00, 0, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for over-cost budget")
	}
	if dec.Reason != ReasonBudgetExceeded {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonBudgetExceeded)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
}

func TestCheckBudget_OverCallsDenies(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	today := time.Now().Local().Format("2006-01-02")
	if _, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		"marketing", today, 0.0, 100, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for over-call budget")
	}
	if dec.Reason != ReasonBudgetExceeded {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonBudgetExceeded)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
}

func TestCheckBudget_DeniedCallDoesNotIncrement(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "evil-plugin/drop-database",
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected trust-check denial")
	}
	today := time.Now().Local().Format("2006-01-02")
	var calls int
	row := db.QueryRow(
		`SELECT call_count FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	)
	if err := row.Scan(&calls); err == nil && calls != 0 {
		t.Errorf("denied call should not increment, got call_count=%d", calls)
	}
}

// TestCheckBudget_YesterdayRowDoesNotBleed confirms at the integration level
// (through PEP.Invoke) that a heavy-usage row for yesterday's date does not
// affect today's budget evaluation.
func TestCheckBudget_YesterdayRowDoesNotBleed(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	yesterday := time.Now().Local().AddDate(0, 0, -1).Format("2006-01-02")
	// Heavy usage that would deny today if it bled in.
	if _, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		"marketing", yesterday, 1000.00, 5000, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("yesterday's usage should not affect today: got reason %q", dec.Reason)
	}
}

func TestCheckBudget_MCPErrorStillIncrements(t *testing.T) {
	mcpc := &fakeMCP{callErr: errors.New("boom")}
	p, db := newTestPEP(t, mcpc)
	_, _, _ = p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	today := time.Now().Local().Format("2006-01-02")
	var calls int
	if err := db.QueryRow(
		`SELECT call_count FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	).Scan(&calls); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if calls != 1 {
		t.Errorf("mcp_error outcome should still bump call_count, got %d want 1", calls)
	}
}
