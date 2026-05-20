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

// Sentinel: after the first call on an envelope without input_schema, mutate
// the envelope JSON for subsequent calls (same key). The second call should
// still return (nil, nil) and ignore the mutated payload — proving the nil
// result is cached and the JSON envelope is not re-unmarshaled.
func TestSchemaCache_CachesNilResultForEnvelopeWithoutInputSchema(t *testing.T) {
	c := &schemaCache{}
	s, err := c.compileOrGet("test/ability", "h1", envelopeWithoutInputSchema)
	if err != nil {
		t.Fatalf("first compileOrGet: %v", err)
	}
	if s != nil {
		t.Fatalf("expected nil schema on first call, got %#v", s)
	}
	// Second call with the same key but a malformed payload — if the function
	// re-entered the parse path it would return an error. Cached nil short-
	// circuits before unmarshal.
	s2, err := c.compileOrGet("test/ability", "h1", "{ this is not valid json")
	if err != nil {
		t.Fatalf("second compileOrGet hit parse path; expected cached nil: %v", err)
	}
	if s2 != nil {
		t.Errorf("expected nil schema on cached call, got %#v", s2)
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
