package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the .tfoutdated.yaml configuration file.
type Config struct {
	Ignore    []IgnoreRule `yaml:"ignore"`
	Severity  string       `yaml:"severity"`
	Providers []string     `yaml:"providers"`
}

// IgnoreRule defines a path or module to ignore during scanning.
type IgnoreRule struct {
	Path   string `yaml:"path"`
	Module string `yaml:"module"`
}

// Load reads the .tfoutdated.yaml config from the given directory.
// Returns a default config if the file doesn't exist.
func Load(dir string) Config {
	cfg := Config{
		Severity: "patch",
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tfoutdated.yaml"))
	if err != nil {
		return cfg
	}

	_ = yaml.Unmarshal(data, &cfg)
	return cfg
}
