package pep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// schemaCache compiles JSON Schemas once per (ability_name, schema_hash) pair
// and reuses the compiled form on subsequent Invokes. Compile is O(schema
// size); validation is O(args size). Moving compile out of the hot path is
// the whole point of the cache.
//
// Backed by sync.Map for concurrent Invoke safety. No eviction in V1:
// manifest is ~20 entries, operator-approved abilities scale slowly,
// compiled schemas are kilobytes.
type schemaCache struct {
	entries sync.Map // map[string]*jsonschema.Schema, key = name + ":" + hash
}

// compileOrGet returns the compiled input_schema for (name, hash). schemaJSON
// is the full canonicalized envelope from abilities.schema_json — the
// function extracts the "input_schema" sub-document before compiling.
// Returns (nil, nil) when the envelope has no input_schema (caller should
// pass through validation).
func (c *schemaCache) compileOrGet(name, hash, schemaJSON string) (*jsonschema.Schema, error) {
	key := name + ":" + hash
	if v, ok := c.entries.Load(key); ok {
		return v.(*jsonschema.Schema), nil
	}
	var envelope struct {
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &envelope); err != nil {
		return nil, fmt.Errorf("decode schema envelope: %w", err)
	}
	if len(envelope.InputSchema) == 0 {
		return nil, nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(key, bytes.NewReader(envelope.InputSchema)); err != nil {
		return nil, fmt.Errorf("add resource: %w", err)
	}
	s, err := compiler.Compile(key)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	c.entries.Store(key, s)
	return s, nil
}
