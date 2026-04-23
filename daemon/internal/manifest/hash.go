package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SchemaHash returns the canonical schema hash for an ability given its raw
// input and output schemas. Formatted as "sha256:<hex>".
//
// The canonicalization rules: object keys are sorted lexicographically,
// arrays preserve their order (JSON schema arrays are semantically ordered),
// numbers are emitted in shortest decimal form, and no extraneous whitespace
// is added. This ensures that semantically-equivalent schemas from different
// plugin releases hash the same, and any field addition/rename/retyping
// produces a different hash.
//
// Both inputSchema and outputSchema are JSON-encoded bytes as returned by
// the Abilities API; passing []byte("null") is valid for abilities with no
// schema on either side.
func SchemaHash(inputSchema, outputSchema []byte) (string, error) {
	in, err := canonicalBytes(inputSchema)
	if err != nil {
		return "", fmt.Errorf("input_schema: %w", err)
	}
	out, err := canonicalBytes(outputSchema)
	if err != nil {
		return "", fmt.Errorf("output_schema: %w", err)
	}
	// Hash a fixed framing so "input nil, output X" hashes distinctly from
	// "input X, output nil".
	h := sha256.New()
	h.Write([]byte("input:"))
	h.Write(in)
	h.Write([]byte("\noutput:"))
	h.Write(out)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// canonicalBytes reads a JSON value and returns its canonical byte form.
// Accepts empty or nil input as JSON null.
func canonicalBytes(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("null"), nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	var buf strings.Builder
	if err := canonicalEncode(&buf, v); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func canonicalEncode(buf *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		// Use encoding/json for string escaping to match RFC 8259 exactly.
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	case float64:
		// Shortest decimal form; preserves integer-looking floats without a
		// trailing .0, and avoids scientific notation for small values.
		buf.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
	case json.Number:
		buf.WriteString(t.String())
	case []any:
		buf.WriteByte('[')
		for i, el := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := canonicalEncode(buf, el); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := canonicalEncode(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("unsupported JSON value type %T", v)
	}
	return nil
}
