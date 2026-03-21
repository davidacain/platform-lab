package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Clusters []ClusterConfig `yaml:"clusters"`
}

type ClusterConfig struct {
	ArgoCDCluster string `yaml:"argocd_cluster"`
	Prometheus    string `yaml:"prometheus"`
}

// Load reads the kri config file. If path is empty, defaults to ~/.kri/config.yaml.
func Load(path string) (*Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		path = filepath.Join(home, ".kri", "config.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

// PrometheusFor returns the Prometheus URL for the given ArgoCD cluster name.
func (c *Config) PrometheusFor(clusterName string) (string, bool) {
	for _, cl := range c.Clusters {
		if cl.ArgoCDCluster == clusterName {
			return cl.Prometheus, true
		}
	}
	return "", false
}
