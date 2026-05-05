package registry

import (
	"path/filepath"
	"testing"
)

func TestLoadSkills_PricingBenchmarkPresent(t *testing.T) {
	dir, err := filepath.Abs("../../skills")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	skills, err := LoadSkills(dir)
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	s, ok := skills["pricing.benchmark"]
	if !ok {
		t.Fatalf("pricing.benchmark not loaded; got: %v", keys(skills))
	}
	if s.Version == "" {
		t.Errorf("pricing.benchmark missing version")
	}
	if s.Description == "" {
		t.Errorf("pricing.benchmark missing description (GEPA optimization target)")
	}
	if s.ContentSHA == "" {
		t.Errorf("pricing.benchmark content_sha not derived")
	}
	if _, ok := s.Schema["input"]; !ok {
		t.Errorf("pricing.benchmark schema.input missing")
	}
	if _, ok := s.Schema["output"]; !ok {
		t.Errorf("pricing.benchmark schema.output missing")
	}
}

func keys(m map[string]Skill) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
