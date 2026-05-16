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
