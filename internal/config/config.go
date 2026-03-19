package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	IdeasPath       string
	UploadsDir      string
	PersonalPath    string
	FamilyPath      string
	UserDataDir     string
	DBPath          string
	APIToken        string
	Addr            string
	PasswordHash    string
	SessionLifetime time.Duration
	SecureCookies   bool
	HasUsers        bool // Set at startup after checking the users table.
	Location        *time.Location
}

func Load() (*Config, error) {
	sessionLifetime, err := time.ParseDuration(envOr("SESSION_LIFETIME", "720h"))
	if err != nil {
		return nil, fmt.Errorf("parsing SESSION_LIFETIME: %w", err)
	}

	secureCookies := true
	if v, ok := os.LookupEnv("DASHBOARD_SECURE_COOKIES"); ok {
		secureCookies, _ = strconv.ParseBool(v)
	}

	loc := time.Local
	if tz := os.Getenv("DASHBOARD_TIMEZONE"); tz != "" {
		var err error
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("parsing DASHBOARD_TIMEZONE %q: %w", tz, err)
		}
	}

	// IDEAS_PATH takes precedence. Fall back to IDEAS_DIR for backwards
	// compatibility: if IDEAS_DIR is set, derive the file path from it.
	ideasPath := os.Getenv("IDEAS_PATH")
	if ideasPath == "" {
		ideasDir := os.Getenv("IDEAS_DIR")
		if ideasDir != "" {
			ideasPath = filepath.Join(filepath.Dir(ideasDir), "ideas.md")
		} else {
			ideasPath = "/data/ideas.md"
		}
	}

	c := &Config{
		IdeasPath:       ideasPath,
		UploadsDir:      envOr("UPLOADS_DIR", "/data/uploads"),
		PersonalPath:    envOr("PERSONAL_PATH", "/data/personal.md"),
		FamilyPath:      envOr("FAMILY_PATH", "/data/family.md"),
		UserDataDir:     envOr("USER_DATA_DIR", "/data/users"),
		DBPath:          envOr("DB_PATH", "/data/db/dashboard.db"),
		APIToken:        os.Getenv("DASHBOARD_API_TOKEN"),
		Addr:            envOr("ADDR", ":8080"),
		PasswordHash:    os.Getenv("DASHBOARD_PASSWORD_HASH"),
		SessionLifetime: sessionLifetime,
		SecureCookies:   secureCookies,
		Location:        loc,
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	if err := os.MkdirAll(c.UploadsDir, 0o755); err != nil {
		return fmt.Errorf("creating uploads dir %q: %w", c.UploadsDir, err)
	}

	// Create skeleton files for tracker markdown files and ideas.
	for _, entry := range []struct {
		path    string
		heading string
	}{
		{c.PersonalPath, "Personal"},
		{c.FamilyPath, "Family"},
		{c.IdeasPath, "Ideas"},
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

// AuthEnabled returns true if authentication should be enforced.
func (c *Config) AuthEnabled() bool {
	return c.PasswordHash != "" || c.HasUsers
}
