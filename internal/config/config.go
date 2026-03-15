package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ProjectsDir string
	IdeasDir    string
	TrackerPath string
	DBPath      string
	APIToken    string
	Addr        string
}

func Load() (*Config, error) {
	c := &Config{
		ProjectsDir: envOr("PROJECTS_DIR", "/data/projects"),
		IdeasDir:    envOr("IDEAS_DIR", "/data/ideas"),
		TrackerPath: envOr("TRACKER_PATH", "/data/tracker.md"),
		DBPath:      envOr("DB_PATH", "/data/db/dashboard.db"),
		APIToken:    os.Getenv("DASHBOARD_API_TOKEN"),
		Addr:        envOr("ADDR", ":8080"),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	if info, err := os.Stat(c.ProjectsDir); err != nil || !info.IsDir() {
		return fmt.Errorf("PROJECTS_DIR %q is not a valid directory", c.ProjectsDir)
	}

	// Ensure ideas subdirectories exist.
	for _, sub := range []string{"untriaged", "parked", "dropped", "research"} {
		dir := filepath.Join(c.IdeasDir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating ideas dir %q: %w", dir, err)
		}
	}

	// Ensure tracker parent directory exists and create skeleton if missing.
	if err := os.MkdirAll(filepath.Dir(c.TrackerPath), 0o755); err != nil {
		return fmt.Errorf("creating tracker directory: %w", err)
	}
	if _, err := os.Stat(c.TrackerPath); os.IsNotExist(err) {
		skeleton := "# Tracker\n\n## General\n"
		if err := os.WriteFile(c.TrackerPath, []byte(skeleton), 0o644); err != nil {
			return fmt.Errorf("creating tracker.md skeleton: %w", err)
		}
	}

	// Ensure DB parent directory exists.
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
