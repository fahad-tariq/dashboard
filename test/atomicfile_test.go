package test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/atomicfile"
	"github.com/fahad/dashboard/internal/config"
)

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestAtomicWriteReplacesContentAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "personal.md")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(path, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new content" {
		t.Fatalf("content = %q (%v)", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want the target's existing 0640", info.Mode().Perm())
	}
	if names := listDir(t, dir); len(names) != 1 {
		t.Errorf("directory has %v, want only personal.md", names)
	}

	fresh := filepath.Join(dir, "new.md")
	if err := atomicfile.Write(fresh, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(fresh); info.Mode().Perm() != 0o644 {
		t.Errorf("new file mode = %v, want default 0644", info.Mode().Perm())
	}
}

// A crash or error at any step must leave the original bytes untouched and
// no temp file behind.
func TestAtomicWriteFailureLeavesOriginal(t *testing.T) {
	boom := errors.New("injected failure")
	tests := map[string]atomicfile.Writer{
		"after write": {
			WriteData: func(f *os.File, data []byte) error {
				if _, err := f.Write(data[:len(data)/2]); err != nil {
					return err
				}
				return boom
			},
		},
		"at fsync":  {Sync: func(*os.File) error { return boom }},
		"at rename": {Rename: func(string, string) error { return boom }},
	}
	for name, w := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "family.md")
			original := []byte("# Family\n\n- [ ] Original task\n")
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			err := w.Write(path, []byte("# Family\n\n- [ ] Replacement that must not land\n"), 0o644)
			if !errors.Is(err, boom) {
				t.Fatalf("Write error = %v, want the injected failure", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Errorf("original changed to %q", got)
			}
			if names := listDir(t, dir); len(names) != 1 {
				t.Errorf("temp file left behind: %v", names)
			}
		})
	}
}

func TestAtomicCleanStaleRemovesOnlyOurTempFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "users", "1")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := []string{
		filepath.Join(dir, "family.md"),
		filepath.Join(dir, ".hidden-notes"),
		filepath.Join(sub, "personal.md"),
		filepath.Join(sub, "draft.tmp"),
	}
	stale := []string{
		filepath.Join(dir, ".family.md.atomic-123.tmp"),
		filepath.Join(sub, ".personal.md.atomic-abc.tmp"),
	}
	for _, p := range append(append([]string{}, keep...), stale...) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := atomicfile.CleanStale(dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range keep {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed", p)
		}
	}
	for _, p := range stale {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("stale temp %s survived", p)
		}
	}
}

// Readers hammering a file while it is rewritten must only ever see a
// complete old or new version. Needs real fsync timing, so it is gated.
func TestAtomicWriteConcurrentReadersNeverSeePartial(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "personal.md")
	a := []byte("# Personal\n\n" + strings.Repeat("- [ ] Alpha task\n", 20000))
	b := []byte("# Personal\n\n" + strings.Repeat("- [x] Bravo task, longer line\n", 15000))
	if err := os.WriteFile(path, a, 0o644); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var bad atomic.Int64
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				got, err := os.ReadFile(path)
				if err != nil || (!bytes.Equal(got, a) && !bytes.Equal(got, b)) {
					bad.Add(1)
				}
			}
		}()
	}
	deadline := time.Now().Add(3 * time.Second)
	for i := 0; time.Now().Before(deadline); i++ {
		data := a
		if i%2 == 0 {
			data = b
		}
		if err := atomicfile.Write(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stop.Store(true)
	wg.Wait()
	if n := bad.Load(); n > 0 {
		t.Errorf("readers saw %d partial or missing reads", n)
	}
}

func TestConfigLoadRemovesStaleTempFiles(t *testing.T) {
	paths := tempPaths(t)
	stale := filepath.Join(filepath.Dir(paths["FAMILY_PATH"]), ".family.md.atomic-999.tmp")
	if err := os.WriteFile(stale, []byte("half a write"), 0o644); err != nil {
		t.Fatal(err)
	}
	setEnvForConfig(t, paths)
	if _, err := config.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("config.Load left a stale temp file in place")
	}
}
