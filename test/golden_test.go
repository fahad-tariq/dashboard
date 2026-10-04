package test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files in testdata/")

// assertGolden compares got against testdata/<name>, rewriting the file when
// the test binary runs with -update.
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s (run with -update to create): %v", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s mismatch (run with -update to accept)\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
