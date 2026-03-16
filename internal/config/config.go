package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	IdeasDir       string
	ExplorationDir string
	UploadsDir     string
	PersonalPath   string
	FamilyPath     string
	DBPath         string
	APIToken       string
	Addr           string
}

func Load() (*Config, error) {
	c := &Config{
		IdeasDir:       envOr("IDEAS_DIR", "/data/ideas"),
		ExplorationDir: envOr("EXPLORATION_DIR", "/data/explorations"),
		UploadsDir:     envOr("UPLOADS_DIR", "/data/uploads"),
		PersonalPath:   envOr("PERSONAL_PATH", "/data/personal.md"),
		FamilyPath:     envOr("FAMILY_PATH", "/data/family.md"),
		DBPath:         envOr("DB_PATH", "/data/db/dashboard.db"),
		APIToken:       os.Getenv("DASHBOARD_API_TOKEN"),
		Addr:           envOr("ADDR", ":8080"),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	for _, sub := range []string{"untriaged", "parked", "dropped", "research"} {
		dir := filepath.Join(c.IdeasDir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating ideas dir %q: %w", dir, err)
		}
	}

	if err := os.MkdirAll(c.ExplorationDir, 0o755); err != nil {
		return fmt.Errorf("creating exploration dir %q: %w", c.ExplorationDir, err)
	}

	if err := os.MkdirAll(c.UploadsDir, 0o755); err != nil {
		return fmt.Errorf("creating uploads dir %q: %w", c.UploadsDir, err)
	}

	for _, entry := range []struct {
		path    string
		heading string
	}{
		{c.PersonalPath, "Personal"},
		{c.FamilyPath, "Family"},
	} {
		if err := os.MkdirAll(filepath.Dir(entry.path), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", entry.path, err)
		}
		if _, err := os.Stat(entry.path); os.IsNotExist(err) {
			skeleton := "# " + entry.heading + "\n\n"
			if err := os.WriteFile(entry.path, []byte(skeleton), 0o644); err != nil {
				return fmt.Errorf("creating %s skeleton: %w", entry.path, err)
			}
		}
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
