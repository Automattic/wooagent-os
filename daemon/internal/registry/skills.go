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

// Skills resolves the canonical skill registry. The embedded `skills/` tree
// inside the binary is the default; `WOOAGENT_SKILLS_DIR` overrides for
// local iteration without rebuilding. Missing-directory and missing-files
// are not errors — the loader returns an empty map. Callers check for the
// specific skill they need and skip when absent.
func Skills() (map[string]Skill, error) {
	if dir := strings.TrimSpace(os.Getenv("WOOAGENT_SKILLS_DIR")); dir != "" {
		return loadFromDisk(dir)
	}
	return loadFromFS(skillsEmbed, "skills")
}

// LoadSkills retains the old disk-path signature for tests and any
// pre-embed callers. New code should prefer Skills().
func LoadSkills(dir string) (map[string]Skill, error) {
	return loadFromDisk(dir)
}

func loadFromDisk(dir string) (map[string]Skill, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return map[string]Skill{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills path %s is not a directory", dir)
	}
	return walkSkills(os.DirFS(dir), ".")
}

func loadFromFS(fsys fs.FS, root string) (map[string]Skill, error) {
	// embed.FS returns an "open ." that lists the root entries; missing
	// roots are an error from embed but the loader is tolerant either way.
	if _, err := fs.Stat(fsys, root); err != nil {
		if os.IsNotExist(err) {
			return map[string]Skill{}, nil
		}
		return nil, err
	}
	return walkSkills(fsys, root)
}

func walkSkills(fsys fs.FS, root string) (map[string]Skill, error) {
	out := map[string]Skill{}
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		body, err := fs.ReadFile(fsys, path)
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
	// filepath is unused on the embed path but keep the dependency for
	// any future os.PathSeparator handling without breaking compile.
	_ = filepath.Separator
	return out, nil
}
