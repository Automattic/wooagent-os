package marketing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVariants_Happy(t *testing.T) {
	raw := `{"variants":[
		{"label":"A","angle":"material","body":"Cotton canvas, hand-dyed in small batches with natural indigo."},
		{"label":"B","angle":"use","body":"Drapes well across a sofa or bed. Made to last through years of use."},
		{"label":"C","angle":"story","body":"Hand-dyed by a small studio in Oaxaca. Each piece carries its own slight variation."}
	]}`
	vs, err := parseVariants(raw)
	if err != nil {
		t.Fatalf("parseVariants: %v", err)
	}
	if len(vs) != 3 {
		t.Fatalf("want 3 variants, got %d", len(vs))
	}
	if vs[0].ID != "var_a" || vs[1].ID != "var_b" || vs[2].ID != "var_c" {
		t.Errorf("ids = %q,%q,%q; want var_a,var_b,var_c", vs[0].ID, vs[1].ID, vs[2].ID)
	}
	if !vs[0].Recommended || vs[1].Recommended || vs[2].Recommended {
		t.Errorf("only the first variant should be recommended")
	}
	if vs[0].CharCount != len(vs[0].Body) {
		t.Errorf("charCount %d != body len %d", vs[0].CharCount, len(vs[0].Body))
	}
}

func TestParseVariants_TolerantOfChatter(t *testing.T) {
	raw := "Here is the JSON:\n```\n{\"variants\":[{\"label\":\"A\",\"body\":\"one\"},{\"label\":\"B\",\"body\":\"two\"},{\"label\":\"C\",\"body\":\"three\"}]}\n```\nLet me know if you want adjustments."
	vs, err := parseVariants(raw)
	if err != nil {
		t.Fatalf("parseVariants: %v", err)
	}
	if len(vs) != 3 {
		t.Fatalf("want 3 variants, got %d", len(vs))
	}
}

func TestParseVariants_RejectsWrongCount(t *testing.T) {
	raw := `{"variants":[{"label":"A","body":"only one"}]}`
	if _, err := parseVariants(raw); err == nil {
		t.Errorf("expected error on 1-variant payload, got nil")
	}
	raw = `{"variants":[]}`
	if _, err := parseVariants(raw); err == nil {
		t.Errorf("expected error on empty variants, got nil")
	}
}

func TestParseVariants_RejectsEmptyBody(t *testing.T) {
	raw := `{"variants":[{"label":"A","body":""},{"label":"B","body":"two"},{"label":"C","body":"three"}]}`
	if _, err := parseVariants(raw); err == nil || !strings.Contains(err.Error(), "empty body") {
		t.Errorf("expected empty-body error, got %v", err)
	}
}

func TestParseVariants_RejectsNonJSON(t *testing.T) {
	raw := "Sorry, I cannot complete this task."
	if _, err := parseVariants(raw); err == nil {
		t.Errorf("expected error on non-JSON output, got nil")
	}
}

func TestProductJSON_ImageFields(t *testing.T) {
	payload := []byte(`{
		"id": 42,
		"name": "Test Product",
		"sku": "SKU-42",
		"status": "publish",
		"description": "",
		"short_description": "",
		"permalink": "",
		"image_url": "https://store.example.com/wp-content/uploads/2024/01/test.jpg",
		"image_alt": "Test product alt text"
	}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "https://store.example.com/wp-content/uploads/2024/01/test.jpg" {
		t.Errorf("ImageURL = %q, want full URL", p.ImageURL)
	}
	if p.ImageAlt != "Test product alt text" {
		t.Errorf("ImageAlt = %q, want full alt", p.ImageAlt)
	}
}

func TestProductJSON_ImageFieldsAbsent_DefaultsEmpty(t *testing.T) {
	payload := []byte(`{"id":1,"name":"x","sku":"","status":"publish","description":"","short_description":"","permalink":""}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "" {
		t.Errorf("ImageURL = %q, want empty string", p.ImageURL)
	}
	if p.ImageAlt != "" {
		t.Errorf("ImageAlt = %q, want empty string", p.ImageAlt)
	}
}

func TestParseVariants_ValidScores(t *testing.T) {
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":80,"voice":75},
		{"label":"B","angle":"use","body":"world","seo":65,"voice":90},
		{"label":"C","angle":"story","body":"again","seo":100,"voice":50}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 80 || got[0].Voice != 75 {
		t.Errorf("variant 0: got seo=%d voice=%d, want 80/75", got[0].Seo, got[0].Voice)
	}
	if got[1].Seo != 65 || got[1].Voice != 90 {
		t.Errorf("variant 1: got seo=%d voice=%d, want 65/90", got[1].Seo, got[1].Voice)
	}
	if got[2].Seo != 100 || got[2].Voice != 50 {
		t.Errorf("variant 2: got seo=%d voice=%d, want 100/50", got[2].Seo, got[2].Voice)
	}
}

func TestParseVariants_OutOfRangeScoresClearedToZero(t *testing.T) {
	// Out-of-range scores → cleared to zero so omitempty drops them from
	// the persisted JSON. UI then renders `—` instead of a misleading number.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":101,"voice":-5},
		{"label":"B","angle":"use","body":"world","seo":50,"voice":50},
		{"label":"C","angle":"story","body":"again","seo":50,"voice":50}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 {
		t.Errorf("variant 0 out-of-range seo: got %d, want 0", got[0].Seo)
	}
	if got[0].Voice != 0 {
		t.Errorf("variant 0 out-of-range voice: got %d, want 0", got[0].Voice)
	}
}

func TestParseVariants_MissingScoresAreZero(t *testing.T) {
	// Missing seo/voice fields → zero-value, which omitempty drops.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello"},
		{"label":"B","angle":"use","body":"world"},
		{"label":"C","angle":"story","body":"again"}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 || got[0].Voice != 0 {
		t.Errorf("missing scores: got seo=%d voice=%d, want 0/0", got[0].Seo, got[0].Voice)
	}
}

func TestParseVariants_ZeroScoreIsClearedIndependently(t *testing.T) {
	// Each score is validated independently — an explicit 0 on one field
	// is cleared regardless of the other field's value. This is distinct
	// from TestParseVariants_MissingScoresAreZero, which covers absent
	// fields; here the LLM explicitly emitted 0 and the validator treats
	// that as the legacy "no scoring" default per DSGWOO-1326.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":0,"voice":0},
		{"label":"B","angle":"use","body":"world","seo":80,"voice":80},
		{"label":"C","angle":"story","body":"again","seo":0,"voice":75}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 || got[0].Voice != 0 {
		t.Errorf("variant 0 both-zero: got seo=%d voice=%d, want 0/0", got[0].Seo, got[0].Voice)
	}
	if got[1].Seo != 80 || got[1].Voice != 80 {
		t.Errorf("variant 1 should keep its scores: got seo=%d voice=%d, want 80/80", got[1].Seo, got[1].Voice)
	}
	if got[2].Seo != 0 {
		t.Errorf("variant 2 asymmetric seo=0: got seo=%d, want 0", got[2].Seo)
	}
	if got[2].Voice != 75 {
		t.Errorf("variant 2 asymmetric voice=75: got voice=%d, want 75 (should NOT be cleared)", got[2].Voice)
	}
}
