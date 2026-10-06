package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildDashboard compiles the dashboard binary into a temp dir, for testing
// its CLI subcommands.
func buildDashboard(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "dashboard")
	out, err := exec.Command("go", "build", "-o", bin, "../cmd/dashboard").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// migrate-data writes a fresh ideas.md from the legacy layout. It must never
// replace an ideas.md that already holds ideas, which is what running it
// against current data would do.
func TestMigrateDataNeverOverwritesExistingIdeas(t *testing.T) {
	bin := buildDashboard(t)
	const current = "# Ideas\n\n- [ ] Keep me [status: parked] [added: 2026-01-01]\n"

	tests := map[string]struct {
		existing    string // ideas.md before the run; "" means no file
		legacy      bool   // whether an old-format idea exists to migrate
		wantErr     bool
		wantContent string // substring expected in ideas.md afterwards
	}{
		"current data, nothing to migrate": {existing: current, wantErr: true, wantContent: "Keep me"},
		"current data and legacy files":    {existing: current, legacy: true, wantErr: true, wantContent: "Keep me"},
		"skeleton file and legacy files":   {existing: "# Ideas\n", legacy: true, wantContent: "Old idea"},
		"no file and legacy files":         {legacy: true, wantContent: "Old idea"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			paths := tempPaths(t)
			userDir := filepath.Join(paths["USER_DATA_DIR"], "1")
			if err := os.MkdirAll(userDir, 0o755); err != nil {
				t.Fatal(err)
			}
			ideasPath := filepath.Join(userDir, "ideas.md")
			if tc.existing != "" {
				writeFile(t, ideasPath, tc.existing)
			}
			if tc.legacy {
				dir := filepath.Join(userDir, "ideas", "untriaged")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(dir, "old-idea.md"), "---\ndate: 2025-01-01\n---\n# Old idea\nBody.\n")
			}

			cmd := exec.Command(bin, "migrate-data", "--user-id", "1")
			cmd.Env = os.Environ()
			for k, v := range paths {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			out, err := cmd.CombinedOutput()
			if tc.wantErr && err == nil {
				t.Errorf("migrate-data succeeded, want a refusal\n%s", out)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("migrate-data: %v\n%s", err, out)
			}

			data, err := os.ReadFile(ideasPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.wantContent) {
				t.Errorf("ideas.md = %q, want it to contain %q", data, tc.wantContent)
			}
		})
	}
}
