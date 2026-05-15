package pep

import (
	"testing"
)

const validEnvelope = `{
  "name": "test/ability",
  "input_schema": {
    "type": "object",
    "properties": {"id": {"type": "integer"}},
    "required": ["id"]
  }
}`

const envelopeWithoutInputSchema = `{"name": "test/ability"}`

const malformedSchemaEnvelope = `{
  "name": "test/ability",
  "input_schema": {"type": "not-a-real-type"}
}`

func TestSchemaCache_CompilesValidSchema(t *testing.T) {
	c := &schemaCache{}
	s, err := c.compileOrGet("test/ability", "h1", validEnvelope)
	if err != nil {
		t.Fatalf("compileOrGet: %v", err)
	}
	if s == nil {
		t.Fatal("expected compiled schema, got nil")
	}
	if err := s.Validate(map[string]any{"id": 1}); err != nil {
		t.Errorf("expected valid args to pass, got %v", err)
	}
	if err := s.Validate(map[string]any{}); err == nil {
		t.Error("expected missing required to fail")
	}
}

func TestSchemaCache_ReturnsNilWhenNoInputSchema(t *testing.T) {
	c := &schemaCache{}
	s, err := c.compileOrGet("test/ability", "h1", envelopeWithoutInputSchema)
	if err != nil {
		t.Fatalf("compileOrGet: %v", err)
	}
	if s != nil {
		t.Errorf("expected nil schema for envelope without input_schema, got %#v", s)
	}
}

func TestSchemaCache_ErrorsOnMalformedSchema(t *testing.T) {
	c := &schemaCache{}
	_, err := c.compileOrGet("test/ability", "h1", malformedSchemaEnvelope)
	if err == nil {
		t.Fatal("expected compile error for invalid schema type")
	}
}

func TestSchemaCache_CachesByNameAndHash(t *testing.T) {
	c := &schemaCache{}
	s1, err := c.compileOrGet("test/ability", "h1", validEnvelope)
	if err != nil {
		t.Fatalf("first compileOrGet: %v", err)
	}
	// Same key → same compiled instance.
	s2, err := c.compileOrGet("test/ability", "h1", validEnvelope)
	if err != nil {
		t.Fatalf("second compileOrGet: %v", err)
	}
	if s1 != s2 {
		t.Error("expected same *jsonschema.Schema pointer for same (name, hash) key")
	}
	// Different hash → new compilation, different pointer.
	s3, err := c.compileOrGet("test/ability", "h2", validEnvelope)
	if err != nil {
		t.Fatalf("third compileOrGet: %v", err)
	}
	if s1 == s3 {
		t.Error("expected different *jsonschema.Schema pointer for different hash")
	}
}
