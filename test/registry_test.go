package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/services"
)

func TestEnsureUserDirsCreatesStructure(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	if err := os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	houseProjectsPath := filepath.Join(tmpDir, "house-projects.md")
	if err := os.WriteFile(houseProjectsPath, []byte("# House\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := services.NewRegistry(database, userDataDir, familyPath, houseProjectsPath, filepath.Join(t.TempDir(), "maintenance.md"), time.UTC)

	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatalf("EnsureUserDirs: %v", err)
	}

	// Verify directory structure.
	expected := []string{
		"1",
		"1/personal.md",
		"1/ideas.md",
	}
	for _, rel := range expected {
		path := filepath.Join(userDataDir, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", rel)
		}
	}
}

func TestEnsureUserDirsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	if err := os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	houseProjectsPath := filepath.Join(tmpDir, "house-projects.md")
	if err := os.WriteFile(houseProjectsPath, []byte("# House\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := services.NewRegistry(database, userDataDir, familyPath, houseProjectsPath, filepath.Join(t.TempDir(), "maintenance.md"), time.UTC)

	// Write some content to personal.md.
	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatalf("first EnsureUserDirs: %v", err)
	}
	personalPath := filepath.Join(userDataDir, "1", "personal.md")
	if err := os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] My task\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Call again -- should not overwrite existing personal.md.
	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatalf("second EnsureUserDirs: %v", err)
	}
	data, _ := os.ReadFile(personalPath)
	if string(data) != "# Personal\n\n- [ ] My task\n" {
		t.Error("EnsureUserDirs should not overwrite existing personal.md")
	}
}

func TestForUserReturnsCachedInstances(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	if err := os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	houseProjectsPath := filepath.Join(tmpDir, "house-projects.md")
	if err := os.WriteFile(houseProjectsPath, []byte("# House\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := services.NewRegistry(database, userDataDir, familyPath, houseProjectsPath, filepath.Join(t.TempDir(), "maintenance.md"), time.UTC)
	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatal(err)
	}

	svc1 := reg.ForUser(1)
	svc2 := reg.ForUser(1)
	if svc1 != svc2 {
		t.Error("ForUser should return the same cached instance on second call")
	}
}
