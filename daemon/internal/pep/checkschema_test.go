package pep

import (
	"context"
	"database/sql"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// schemaEnvelopeRequiringID — schema_json shape stored by the abilities
// canonicalizer: a top-level object with an "input_schema" sub-document.
const schemaEnvelopeRequiringID = `{
  "name": "wooagent-products/update",
  "input_schema": {
    "type": "object",
    "properties": {"id": {"type": "integer"}, "description": {"type": "string"}},
    "required": ["id"]
  }
}`

const schemaEnvelopeMalformed = `{
  "name": "wooagent-products/update",
  "input_schema": {"type": "not-a-real-type"}
}`

func insertAbility(t *testing.T, db *sql.DB, name, trustState, schemaJSON, schemaHash string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO abilities(name, trust_state, schema_json, schema_hash) VALUES(?, ?, ?, ?)`,
		name, trustState, schemaJSON, schemaHash,
	)
	if err != nil {
		t.Fatalf("insert ability: %v", err)
	}
}

func TestCheckSchema_PassThroughWhenNoDBRow(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, _ := newTestPEP(t, mcpc)
	// No row inserted for this ability; manifest entry exists.
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
		t.Errorf("expected allowed (pass-through), got denied: %s", dec.Reason)
	}
}

func TestCheckSchema_PassThroughWhenSchemaJSONEmpty(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", "", "")
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
		t.Errorf("expected allowed (empty schema_json passes through), got: %s", dec.Reason)
	}
}

func TestCheckSchema_AllowsValidArgs(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1, "description": "x"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed for valid args, got: %s", dec.Reason)
	}
}

func TestCheckSchema_DeniesMissingRequired(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"description": "no id here"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for missing required field")
	}
	if dec.Reason != ReasonInvalidArguments {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonInvalidArguments)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
}

func TestCheckSchema_DeniesWrongType(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": "not-an-integer"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for wrong type")
	}
	if dec.Reason != ReasonInvalidArguments {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonInvalidArguments)
	}
}

func TestCheckSchema_DeniesOnCompileFailure(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeMalformed, "h1")
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
		t.Fatal("expected denied for malformed schema")
	}
	if dec.Reason != ReasonSchemaCompileError {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonSchemaCompileError)
	}
}

// Sentinel: after the first validating call, mutate schema_json in the DB
// without changing schema_hash. The second call should still validate
// against the original schema — proving the compiled form was cached.
func TestCheckSchema_UsesCachedCompiledSchema(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")

	// First call: warm the cache.
	if _, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	}); err != nil {
		t.Fatalf("warm call: %v", err)
	}

	// Mutate schema_json to require a different field; keep hash the same.
	mutated := `{"name":"x","input_schema":{"type":"object","required":["different"]}}`
	if _, err := db.ExecContext(context.Background(),
		`UPDATE abilities SET schema_json = ? WHERE name = ?`,
		mutated, "wooagent-products/update",
	); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	// Second call with original args; should still pass because the cache
	// holds the compiled (id-required) schema.
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed (cached schema), got: %s", dec.Reason)
	}
}
