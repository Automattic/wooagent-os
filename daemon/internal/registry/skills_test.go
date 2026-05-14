package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// Skills() is the canonical accessor — uses the embedded skills/ tree
// baked into the binary. This is what production code calls.
func TestSkills_PricingBenchmarkPresent(t *testing.T) {
	// Clear the disk-override so we exercise the embed path even when
	// the test is run in an environment with WOOAGENT_SKILLS_DIR set.
	t.Setenv("WOOAGENT_SKILLS_DIR", "")
	skills, err := Skills()
	if err != nil {
		t.Fatalf("Skills: %v", err)
	}
	assertPricingBenchmark(t, skills)
}

// LoadSkills (disk path) keeps working for tests / dev iteration with
// the WOOAGENT_SKILLS_DIR override. We point it at the package-local
// skills/ tree so the test doesn't depend on cwd.
func TestLoadSkills_DiskPath(t *testing.T) {
	dir, err := filepath.Abs("skills")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	skills, err := LoadSkills(dir)
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	assertPricingBenchmark(t, skills)
}

// LoadSkills on a missing directory returns an empty map, not an error.
func TestLoadSkills_MissingDirReturnsEmpty(t *testing.T) {
	skills, err := LoadSkills(filepath.Join(os.TempDir(), "definitely-not-a-skills-dir-xyz"))
	if err != nil {
		t.Fatalf("LoadSkills should tolerate missing dir, got err: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected empty map, got %v", keys(skills))
	}
}

func assertPricingBenchmark(t *testing.T, skills map[string]Skill) {
	t.Helper()
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
