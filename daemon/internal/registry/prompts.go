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

// Prompt is one versioned persona prompt on disk. Matches the YAML schema in
// daemon/prompts/README.md. The registry exists so GEPA (v2) can mutate text
// via file drops without rebuilding the daemon binary — see
// wooagent-os-gepa-v2-plan.md §"v1 scaffolding".
type Prompt struct {
	Name        string `yaml:"name"`
	Persona     string `yaml:"persona"`
	Version     string `yaml:"version"`
	ContentSHA  string `yaml:"content_sha,omitempty"`
	Supersedes  string `yaml:"supersedes,omitempty"`
	Body        string `yaml:"body"`
	SourcePath  string `yaml:"-"`
}

// LoadPrompts walks dir looking for `<persona>/v*.yaml` files and returns the
// latest-version Prompt per persona. Keys in the returned map are persona IDs.
// An empty or missing directory is not an error — v1 ships with no prompts,
// and agents only start reading from the registry once Phase 2 lands.
func LoadPrompts(dir string) (map[string]Prompt, error) {
	out := map[string]Prompt{}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("prompts path %s is not a directory", dir)
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
		var p Prompt
		if err := yaml.Unmarshal(body, &p); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if p.Persona == "" || p.Version == "" {
			return fmt.Errorf("%s: persona and version are required", path)
		}
		p.SourcePath = path
		if p.ContentSHA == "" {
			sum := sha256.Sum256([]byte(p.Body))
			p.ContentSHA = hex.EncodeToString(sum[:])
		}
		cur, ok := out[p.Persona]
		if !ok || p.Version > cur.Version {
			out[p.Persona] = p
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
