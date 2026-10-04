package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/tracker"
)

// BenchmarkMutate200 measures one read-modify-write cycle on a realistic
// 200-item tracker, including the file write and any cache or DB upkeep.
func BenchmarkMutate200(b *testing.B) {
	dir := b.TempDir()
	mdPath := filepath.Join(dir, "personal.md")

	var sb strings.Builder
	sb.WriteString("# Personal\n\n")
	for i := range 200 {
		fmt.Fprintf(&sb, "- [ ] Task number %d !medium [added: 2026-01-02] [tags: home, errands]\n", i)
		sb.WriteString("  Some body text describing the task.\n")
		sb.WriteString("  - [ ] First step\n")
		sb.WriteString("  - [x] Second step\n")
	}
	if err := os.WriteFile(mdPath, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { database.Close() })

	svc := tracker.NewService(mdPath, "Personal", time.UTC)
	if err := svc.Resync(); err != nil {
		b.Fatal(err)
	}
	slug := tracker.Slugify("Task number 100")
	priorities := []string{"high", "low"}

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if err := svc.UpdatePriority(slug, priorities[i%2]); err != nil {
			b.Fatal(err)
		}
		i++
	}
}
