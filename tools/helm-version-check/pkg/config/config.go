package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure for hvc.
type Config struct {
	Repos []RepoConfig `yaml:"repos"`
	Pins  []PinEntry   `yaml:"pins,omitempty"`
}

// PinEntry declares that drift on specific fields is intentional for a release.
// Matched releases are shown as dim (not flagged) with a ~ prefix on pinned columns.
// A row is only dimmed when every drifting attribute is pinned; if any unpinned
// attribute is also off, the row is still flagged.
type PinEntry struct {
	Release      string `yaml:"release"`
	Namespace    string `yaml:"namespace,omitempty"` // empty matches any namespace
	ChartVersion bool   `yaml:"chart_version,omitempty"`
	AppVersion   bool   `yaml:"app_version,omitempty"`
	ImageTag     bool   `yaml:"image_tag,omitempty"`
	Reason       string `yaml:"reason,omitempty"`
}

// FindPin returns the PinEntry for the given release/namespace, or nil if none matches.
func (c Config) FindPin(release, namespace string) *PinEntry {
	for i := range c.Pins {
		p := &c.Pins[i]
		if p.Release != release {
			continue
		}
		if p.Namespace != "" && p.Namespace != namespace {
			continue
		}
		return p
	}
	return nil
}

// RepoConfig describes a single repository provider entry.
type RepoConfig struct {
	Name string     `yaml:"name"`
	Type string     `yaml:"type"` // helm-http, artifactory, oci, chartmuseum
	URL  string     `yaml:"url"`
	Auth AuthConfig `yaml:"auth,omitempty"`
}

// AuthConfig holds optional authentication for a repo.
type AuthConfig struct {
	Type     string `yaml:"type,omitempty"`     // token, basic
	Token    string `yaml:"token,omitempty"`    // supports ${ENV_VAR} interpolation
	Username string `yaml:"username,omitempty"` // supports ${ENV_VAR} interpolation
	Password string `yaml:"password,omitempty"` // supports ${ENV_VAR} interpolation
}

// Load reads the config file from the given path. If path is empty,
// it falls back to ~/.hvc/config.yaml. Returns an empty Config (not an error)
// if the file does not exist.
func Load(path string) (Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("find home dir: %w", err)
		}
		path = filepath.Join(home, ".hvc", "config.yaml")
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	// Expand ${ENV_VAR} references before parsing.
	data = []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
