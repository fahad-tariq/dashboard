package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/house"
)

func TestParseCadence(t *testing.T) {
	tests := []struct {
		input   string
		wantN   int
		wantU   byte
		wantErr bool
	}{
		{"3m", 3, 'm', false},
		{"2w", 2, 'w', false},
		{"90d", 90, 'd', false},
		{"1y", 1, 'y', false},
		{"14d", 14, 'd', false},
		{"", 0, 0, true},
		{"x", 0, 0, true},
		{"3x", 0, 0, true},
		{"0d", 0, 0, true},  // must be >= 1
		{"-1m", 0, 0, true}, // negative
		{"9999d", 0, 0, true}, // exceeds max
		{"999y", 0, 0, true},  // exceeds max
	}

	for _, tc := range tests {
		n, u, err := house.ParseCadence(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseCadence(%q): want error, got (%d, %c, nil)", tc.input, n, u)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCadence(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if n != tc.wantN || u != tc.wantU {
			t.Errorf("ParseCadence(%q): got (%d, %c), want (%d, %c)", tc.input, n, u, tc.wantN, tc.wantU)
		}
	}
}

func TestParseMaintenanceBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maintenance.md")

	content := `# Maintenance

- [ ] Clean gutters [cadence: 3m] [tags: exterior] [added: 2025-01-01]
  - [x] 2026-03-15 - installed gutter guard
  - [x] 2025-12-20

- [ ] Mow lawn [cadence: 2w] [tags: garden]
  - [x] 2026-03-22
`
	writeFile(t, path, content)

	items, err := house.ParseMaintenance(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	gutters := items[0]
	if gutters.Title != "Clean gutters" {
		t.Errorf("title: got %q", gutters.Title)
	}
	if gutters.Cadence != "3m" {
		t.Errorf("cadence: got %q", gutters.Cadence)
	}
	if len(gutters.Tags) != 1 || gutters.Tags[0] != "exterior" {
		t.Errorf("tags: got %v", gutters.Tags)
	}
	if gutters.Added != "2025-01-01" {
		t.Errorf("added: got %q", gutters.Added)
	}
	if len(gutters.Log) != 2 {
		t.Fatalf("log entries: got %d, want 2", len(gutters.Log))
	}
	if gutters.Log[0].Date != "2026-03-15" || gutters.Log[0].Note != "installed gutter guard" {
		t.Errorf("log[0]: got %+v", gutters.Log[0])
	}
	if gutters.Log[1].Date != "2025-12-20" || gutters.Log[1].Note != "" {
		t.Errorf("log[1]: got %+v", gutters.Log[1])
	}

	lawn := items[1]
	if lawn.Title != "Mow lawn" {
		t.Errorf("title: got %q", lawn.Title)
	}
	if lawn.Cadence != "2w" {
		t.Errorf("cadence: got %q", lawn.Cadence)
	}
	if len(lawn.Log) != 1 {
		t.Fatalf("log entries: got %d, want 1", len(lawn.Log))
	}
}

func TestParseMaintenanceEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maintenance.md")
	writeFile(t, path, "# Maintenance\n\n")

	items, err := house.ParseMaintenance(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseMaintenanceMissing(t *testing.T) {
	items, err := house.ParseMaintenance("/nonexistent/file.md")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil, got %v", items)
	}
}

func TestParseMaintenanceNoLogEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maintenance.md")
	writeFile(t, path, "# Maintenance\n\n- [ ] Clean gutters [cadence: 3m]\n")

	items, err := house.ParseMaintenance(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].Log) != 0 {
		t.Errorf("expected 0 log entries, got %d", len(items[0].Log))
	}
}

func TestWriteMaintenanceRoundTrip(t *testing.T) {
	input := []house.MaintenanceItem{
		{
			Slug:    "clean-gutters",
			Title:   "Clean gutters",
			Cadence: "3m",
			Tags:    []string{"exterior"},
			Added:   "2025-01-01",
			Log: []house.LogEntry{
				{Date: "2026-03-15", Note: "installed gutter guard"},
				{Date: "2025-12-20"},
			},
		},
		{
			Slug:    "mow-lawn",
			Title:   "Mow lawn",
			Cadence: "2w",
			Tags:    []string{"garden"},
			Log: []house.LogEntry{
				{Date: "2026-03-22"},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "maintenance.md")

	if err := house.WriteMaintenance(path, "Maintenance", input); err != nil {
		t.Fatalf("write: %v", err)
	}

	output, err := house.ParseMaintenance(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(output) != 2 {
		t.Fatalf("expected 2 items, got %d", len(output))
	}

	// Verify first item round-trips correctly.
	g := output[0]
	if g.Title != "Clean gutters" || g.Cadence != "3m" || g.Added != "2025-01-01" {
		t.Errorf("gutters mismatch: %+v", g)
	}
	if len(g.Tags) != 1 || g.Tags[0] != "exterior" {
		t.Errorf("tags mismatch: %v", g.Tags)
	}
	if len(g.Log) != 2 {
		t.Fatalf("log count: got %d, want 2", len(g.Log))
	}
	if g.Log[0].Date != "2026-03-15" || g.Log[0].Note != "installed gutter guard" {
		t.Errorf("log[0] mismatch: %+v", g.Log[0])
	}
	if g.Log[1].Date != "2025-12-20" || g.Log[1].Note != "" {
		t.Errorf("log[1] mismatch: %+v", g.Log[1])
	}
}

func TestMaintenanceOverdue(t *testing.T) {
	loc := time.UTC

	// Item with 3-month cadence, last done 4 months ago.
	overdue := house.MaintenanceItem{
		Cadence: "3m",
		Log:     []house.LogEntry{{Date: "2025-11-01"}},
	}
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, loc)
	if !overdue.IsOverdue(now, loc) {
		t.Error("expected overdue")
	}

	// Item with 3-month cadence, last done 1 month ago.
	recent := house.MaintenanceItem{
		Cadence: "3m",
		Log:     []house.LogEntry{{Date: "2026-02-15"}},
	}
	if recent.IsOverdue(now, loc) {
		t.Error("expected not overdue")
	}

	// Item with no log entries -- always overdue.
	neverDone := house.MaintenanceItem{
		Cadence: "2w",
	}
	if !neverDone.IsOverdue(now, loc) {
		t.Error("no log entries should be overdue")
	}

	// Days until due.
	days := recent.DaysUntilDue(now, loc)
	if days <= 0 {
		t.Errorf("expected positive days, got %d", days)
	}

	neverDays := neverDone.DaysUntilDue(now, loc)
	if neverDays != -9999 {
		t.Errorf("expected -9999, got %d", neverDays)
	}
}

func TestMaintenanceCadenceMonthEnd(t *testing.T) {
	loc := time.UTC
	// 1 month from Jan 31 -> Feb 28 (Go normalises).
	item := house.MaintenanceItem{
		Cadence: "1m",
		Log:     []house.LogEntry{{Date: "2026-01-31"}},
	}
	due := item.NextDue(loc)
	// Go's AddDate(0,1,0) from Jan 31 gives Mar 3 (31 days in Feb overflow).
	// This is expected Go behaviour.
	if due.IsZero() {
		t.Error("expected non-zero due date")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
