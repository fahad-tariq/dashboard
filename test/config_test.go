package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/config"
)

func setEnvForConfig(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func tempPaths(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	return map[string]string{
		"IDEAS_PATH":    filepath.Join(dir, "ideas.md"),
		"UPLOADS_DIR":   filepath.Join(dir, "uploads"),
		"PERSONAL_PATH": filepath.Join(dir, "personal.md"),
		"FAMILY_PATH":   filepath.Join(dir, "family.md"),
		"USER_DATA_DIR": filepath.Join(dir, "users"),
		"DB_PATH":       filepath.Join(dir, "db", "dashboard.db"),
	}
}

func TestConfigLoadReadsEnvVars(t *testing.T) {
	paths := tempPaths(t)
	paths["ADDR"] = ":9090"
	paths["DASHBOARD_API_TOKEN"] = "test-token"
	setEnvForConfig(t, paths)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Addr != ":9090" {
		t.Errorf("Addr: got %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.APIToken != "test-token" {
		t.Errorf("APIToken: got %q, want %q", cfg.APIToken, "test-token")
	}
	if cfg.IdeasPath != paths["IDEAS_PATH"] {
		t.Errorf("IdeasPath: got %q, want %q", cfg.IdeasPath, paths["IDEAS_PATH"])
	}
}

func TestConfigAuthEnabledWithHashOnly(t *testing.T) {
	cfg := &config.Config{PasswordHash: "somehash", HasUsers: false}
	if !cfg.AuthEnabled() {
		t.Error("AuthEnabled should be true when PasswordHash is set")
	}
}

func TestConfigAuthEnabledWithUsersOnly(t *testing.T) {
	cfg := &config.Config{PasswordHash: "", HasUsers: true}
	if !cfg.AuthEnabled() {
		t.Error("AuthEnabled should be true when HasUsers is true")
	}
}

func TestConfigAuthDisabledWhenNeitherSet(t *testing.T) {
	cfg := &config.Config{PasswordHash: "", HasUsers: false}
	if cfg.AuthEnabled() {
		t.Error("AuthEnabled should be false when neither PasswordHash nor HasUsers is set")
	}
}

func TestConfigSecureCookiesDefaultTrue(t *testing.T) {
	paths := tempPaths(t)
	setEnvForConfig(t, paths)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SecureCookies {
		t.Error("SecureCookies should default to true")
	}
}

func TestConfigSecureCookiesExplicitFalse(t *testing.T) {
	paths := tempPaths(t)
	paths["DASHBOARD_SECURE_COOKIES"] = "false"
	setEnvForConfig(t, paths)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SecureCookies {
		t.Error("SecureCookies should be false when DASHBOARD_SECURE_COOKIES=false")
	}
}

func TestConfigIdeasDirBackwardsCompatibility(t *testing.T) {
	dir := t.TempDir()
	paths := tempPaths(t)
	delete(paths, "IDEAS_PATH")
	paths["IDEAS_DIR"] = filepath.Join(dir, "somedir", "old-ideas")
	setEnvForConfig(t, paths)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := filepath.Join(dir, "somedir", "ideas.md")
	if cfg.IdeasPath != want {
		t.Errorf("IdeasPath: got %q, want %q", cfg.IdeasPath, want)
	}
}

func TestConfigIdeasPathTakesPrecedenceOverDir(t *testing.T) {
	dir := t.TempDir()
	paths := tempPaths(t)
	paths["IDEAS_PATH"] = filepath.Join(dir, "custom-ideas.md")
	paths["IDEAS_DIR"] = filepath.Join(dir, "old-dir", "ignored")
	setEnvForConfig(t, paths)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.IdeasPath != paths["IDEAS_PATH"] {
		t.Errorf("IdeasPath: got %q, want %q (IDEAS_PATH should take precedence over IDEAS_DIR)", cfg.IdeasPath, paths["IDEAS_PATH"])
	}
}

func TestConfigValidateCreatesSkeletonFiles(t *testing.T) {
	paths := tempPaths(t)
	setEnvForConfig(t, paths)

	_, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, path := range []string{paths["PERSONAL_PATH"], paths["FAMILY_PATH"], paths["IDEAS_PATH"]} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected skeleton file at %s: %v", path, err)
		}
	}
}
