package test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/tracker"
)

// A move that cannot write the target list must leave the item in the
// source; deleting first used to lose it.
func TestMoveToListKeepsItemWhenTargetWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	srcDir, dstDir := t.TempDir(), t.TempDir()
	personalPath := filepath.Join(srcDir, "personal.md")
	familyPath := filepath.Join(dstDir, "family.md")
	if err := os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Fix the gate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(srcDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, database) })
	personal := tracker.NewService(personalPath, "Personal", time.UTC)
	family := tracker.NewService(familyPath, "Family", time.UTC)

	// Make the target unwritable, file and directory both.
	if err := os.Chmod(familyPath, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dstDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dstDir, 0o755)
		_ = os.Chmod(familyPath, 0o644)
	})

	h := tracker.NewHandler(personal, family, map[string]*template.Template{}, "todos", time.UTC)
	r := chi.NewRouter()
	r.Post("/todos/{slug}/move", h.MoveToList)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("POST", "/todos/fix-the-gate/move", nil))

	if rr.Code < http.StatusInternalServerError {
		t.Errorf("status = %d, want a server error for the failed move", rr.Code)
	}
	if _, err := personal.Get("fix-the-gate"); err != nil {
		t.Fatalf("item lost from the source after a failed move: %v", err)
	}
	content, err := os.ReadFile(personalPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- [ ] Fix the gate"; !strings.Contains(string(content), want) {
		t.Errorf("personal.md no longer contains %q:\n%s", want, content)
	}
}
