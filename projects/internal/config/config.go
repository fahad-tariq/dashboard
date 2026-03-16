package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ProjectsDir string
	DBPath      string
	Addr        string
}

func Load() (*Config, error) {
	c := &Config{
		ProjectsDir: envOr("PROJECTS_DIR", "/data/projects"),
		DBPath:      envOr("DB_PATH", "/data/db/projects.db"),
		Addr:        envOr("ADDR", ":8080"),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	info, err := os.Stat(c.ProjectsDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("PROJECTS_DIR %q is not a valid directory", c.ProjectsDir)
	}

	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0o755); err != nil {
		return fmt.Errorf("creating DB directory: %w", err)
	}

	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
