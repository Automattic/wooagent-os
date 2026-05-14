package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is persisted in ~/.wooagent/config.yaml. Fields added here must be
// backwards-compatible — operators will carry this file across daemon upgrades.
type Config struct {
	BindAddr string `yaml:"bind_addr"`
	UIURL    string `yaml:"ui_url,omitempty"`
}

func Default() Config {
	return Config{
		BindAddr: "localhost:7777",
	}
}

// Paths describes the on-disk layout of the daemon's state directory.
type Paths struct {
	Root           string // ~/.wooagent
	ConfigFile     string // ~/.wooagent/config.yaml
	DBFile         string // ~/.wooagent/wooagent.db
	LogsDir        string // ~/.wooagent/logs
	UISessionFile  string // ~/.wooagent/ui-session.token — bearer token for the embedded UI; persists across daemon restarts (mode 0600)
}

// DefaultPaths returns the default layout (~/.wooagent). Callers can override
// the root for tests by passing a different base dir to PathsAt.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}
	return PathsAt(filepath.Join(home, ".wooagent")), nil
}

func PathsAt(root string) Paths {
	return Paths{
		Root:          root,
		ConfigFile:    filepath.Join(root, "config.yaml"),
		DBFile:        filepath.Join(root, "wooagent.db"),
		LogsDir:       filepath.Join(root, "logs"),
		UISessionFile: filepath.Join(root, "ui-session.token"),
	}
}

func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.Root, p.LogsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func Load(path string) (Config, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(bytes, &c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if c.BindAddr == "" {
		c.BindAddr = Default().BindAddr
	}
	return c, nil
}

func Save(path string, c Config) error {
	bytes, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// LoadOrDefault returns the config at path, or Default() if the file does not
// exist. Other read errors bubble up.
func LoadOrDefault(path string) (Config, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	return Load(path)
}
