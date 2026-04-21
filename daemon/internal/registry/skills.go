package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is a local (in-daemon) skill definition. Distinct from MCP abilities,
// which are discovered from the store at runtime. See PRD §8.2. The skill
// description is a first-class optimization target for GEPA because it drives
// skill-selection accuracy in the agent harness.
type Skill struct {
	Name        string         `yaml:"name"`
	Version     string         `yaml:"version"`
	ContentSHA  string         `yaml:"content_sha,omitempty"`
	Description string         `yaml:"description"`
	Schema      map[string]any `yaml:"schema,omitempty"`
	Examples    []string       `yaml:"examples,omitempty"`
	SourcePath  string         `yaml:"-"`
}

// LoadSkills walks dir for `<skill-name>/v*.yaml` and returns the latest
// version per skill. Missing dir is fine — v1 ships no skills in the registry.
func LoadSkills(dir string) (map[string]Skill, error) {
	out := map[string]Skill{}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills path %s is not a directory", dir)
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var s Skill
		if err := yaml.Unmarshal(body, &s); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if s.Name == "" || s.Version == "" {
			return fmt.Errorf("%s: name and version are required", path)
		}
		s.SourcePath = path
		if s.ContentSHA == "" {
			sum := sha256.Sum256([]byte(s.Description))
			s.ContentSHA = hex.EncodeToString(sum[:])
		}
		cur, ok := out[s.Name]
		if !ok || s.Version > cur.Version {
			out[s.Name] = s
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
