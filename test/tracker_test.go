package test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fahad/dashboard/internal/tracker"
)

func TestParseTrackerEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(""), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseTrackerMissing(t *testing.T) {
	items, err := tracker.ParseTracker(filepath.Join(t.TempDir(), "nope.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for missing file, got %v", items)
	}
}

func TestParseTrackerSingleTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte("# Tracker\n\n## Work\n\n- [ ] Update resume\n"), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Title != "Update resume" {
		t.Errorf("title: got %q", it.Title)
	}
	if it.Type != tracker.TaskType {
		t.Errorf("type: got %q", it.Type)
	}
	if !it.HasTag("Work") {
		t.Errorf("expected tag 'Work', got %v", it.Tags)
	}
	if it.Done {
		t.Error("expected not done")
	}
}

func TestParseTrackerMultipleSections(t *testing.T) {
	content := `# Tracker

## Health

- [ ] Run 5km !high
- [ ] Drink more water

## Reading

- [ ] Read 40 books [goal: 12/40 books]

## Done

- [x] Set up standing desk
`
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	tests := []struct {
		idx      int
		title    string
		typ      tracker.ItemType
		tag      string
		priority string
		done     bool
		current  float64
		target   float64
		unit     string
	}{
		{0, "Run 5km", tracker.TaskType, "Health", "high", false, 0, 0, ""},
		{1, "Drink more water", tracker.TaskType, "Health", "", false, 0, 0, ""},
		{2, "Read 40 books", tracker.GoalType, "Reading", "", false, 12, 40, "books"},
		{3, "Set up standing desk", tracker.TaskType, "Done", "", true, 0, 0, ""},
	}

	for _, tt := range tests {
		it := items[tt.idx]
		if it.Title != tt.title {
			t.Errorf("[%d] title: got %q, want %q", tt.idx, it.Title, tt.title)
		}
		if it.Type != tt.typ {
			t.Errorf("[%d] type: got %q, want %q", tt.idx, it.Type, tt.typ)
		}
		if !it.HasTag(tt.tag) {
			t.Errorf("[%d] expected tag %q, got %v", tt.idx, tt.tag, it.Tags)
		}
		if it.Priority != tt.priority {
			t.Errorf("[%d] priority: got %q, want %q", tt.idx, it.Priority, tt.priority)
		}
		if it.Done != tt.done {
			t.Errorf("[%d] done: got %v, want %v", tt.idx, it.Done, tt.done)
		}
		if it.Current != tt.current {
			t.Errorf("[%d] current: got %v, want %v", tt.idx, it.Current, tt.current)
		}
		if it.Target != tt.target {
			t.Errorf("[%d] target: got %v, want %v", tt.idx, it.Target, tt.target)
		}
		if it.Unit != tt.unit {
			t.Errorf("[%d] unit: got %q, want %q", tt.idx, it.Unit, tt.unit)
		}
	}
}

func TestParseTrackerGoalWithDecimals(t *testing.T) {
	content := "## Health\n\n- [ ] Reach 90kg [goal: 85.5/90 kg]\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Current != 85.5 {
		t.Errorf("current: got %v, want 85.5", items[0].Current)
	}
	if items[0].Target != 90 {
		t.Errorf("target: got %v, want 90", items[0].Target)
	}
	if items[0].Unit != "kg" {
		t.Errorf("unit: got %q, want %q", items[0].Unit, "kg")
	}
}

func TestParseTrackerGraduatedItem(t *testing.T) {
	content := "## Work\n\n- [ ] Study system design [graduated]\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !items[0].Graduated {
		t.Error("expected graduated=true")
	}
}

func TestParseTrackerItemWithBody(t *testing.T) {
	content := "## Work\n\n- [ ] Finish report !high\n  Draft is in Google Docs\n  Due next Friday\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1, got %d", len(items))
	}
	if items[0].Body != "Draft is in Google Docs\nDue next Friday" {
		t.Errorf("body: got %q", items[0].Body)
	}
}

func TestWriteTrackerRoundTrip(t *testing.T) {
	input := []tracker.Item{
		{Slug: "run-5km", Title: "Run 5km", Type: tracker.TaskType, Tags: []string{"Health"}, Priority: "high"},
		{Slug: "read-40-books", Title: "Read 40 books", Type: tracker.GoalType, Tags: []string{"Reading"}, Current: 12, Target: 40, Unit: "books"},
		{Slug: "set-up-standing-desk", Title: "Set up standing desk", Type: tracker.TaskType, Tags: []string{"Health"}, Done: true},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tracker.md")

	if err := tracker.WriteTracker(path, input); err != nil {
		t.Fatalf("write: %v", err)
	}

	output, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(output) != 3 {
		t.Fatalf("expected 3 items, got %d", len(output))
	}

	bySlug := map[string]tracker.Item{}
	for _, it := range output {
		bySlug[it.Slug] = it
	}

	run := bySlug["run-5km"]
	if run.Title != "Run 5km" || run.Priority != "high" || !run.HasTag("Health") {
		t.Errorf("run-5km mismatch: %+v", run)
	}

	read := bySlug["read-40-books"]
	if read.Current != 12 || read.Target != 40 || read.Unit != "books" || read.Type != tracker.GoalType {
		t.Errorf("read-40-books mismatch: %+v", read)
	}

	desk := bySlug["set-up-standing-desk"]
	if !desk.Done {
		t.Error("set-up-desk should be done")
	}
}

func TestWriteTrackerPreservesBody(t *testing.T) {
	input := []tracker.Item{
		{Slug: "report", Title: "Finish report", Type: tracker.TaskType, Tags: []string{"Work"}, Body: "Draft in Google Docs\nDue Friday"},
	}

	path := filepath.Join(t.TempDir(), "tracker.md")
	if err := tracker.WriteTracker(path, input); err != nil {
		t.Fatalf("write: %v", err)
	}

	output, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(output) != 1 {
		t.Fatalf("expected 1, got %d", len(output))
	}
	if output[0].Body != "Draft in Google Docs\nDue Friday" {
		t.Errorf("body: got %q", output[0].Body)
	}
}

func TestParseQuickAdd(t *testing.T) {
	tests := []struct {
		input    string
		title    string
		tags     []string
		priority string
	}{
		{"Buy groceries #errands !high", "Buy groceries", []string{"errands"}, "high"},
		{"Read chapter 5 #study", "Read chapter 5", []string{"study"}, ""},
		{"Fix the sink !low", "Fix the sink", nil, "low"},
		{"Simple task", "Simple task", nil, ""},
		{"Deploy app #work !medium", "Deploy app", []string{"work"}, "medium"},
		{"Multi tag #tech #study", "Multi tag", []string{"tech", "study"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			item := tracker.ParseQuickAdd(tt.input)
			if item.Title != tt.title {
				t.Errorf("title: got %q, want %q", item.Title, tt.title)
			}
			if !slices.Equal(item.Tags, tt.tags) {
				t.Errorf("tags: got %v, want %v", item.Tags, tt.tags)
			}
			if item.Priority != tt.priority {
				t.Errorf("priority: got %q, want %q", item.Priority, tt.priority)
			}
			if item.Type != tracker.TaskType {
				t.Errorf("type should be task, got %q", item.Type)
			}
		})
	}
}

func TestParseTrackerInlineTags(t *testing.T) {
	content := "- [ ] Learn Go [tags: tech, study]\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1, got %d", len(items))
	}
	if !slices.Equal(items[0].Tags, []string{"tech", "study"}) {
		t.Errorf("tags: got %v, want [tech study]", items[0].Tags)
	}
	if items[0].Title != "Learn Go" {
		t.Errorf("title: got %q, want %q", items[0].Title, "Learn Go")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"Read 40 books", "read-40-books"},
		{"Reach 90kg!", "reach-90kg"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"special/chars\\here", "specialcharshere"},
		{"../path-traversal", "path-traversal"},
		{"../../etc/passwd", "etcpasswd"},
		{"dots.and.more.dots", "dotsandmoredots"},
		{"already-slugified", "already-slugified"},
		{"UPPER_CASE_TITLE", "upper-case-title"},
		{"---leading-trailing---", "leading-trailing"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tracker.Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTrackerDoneSection(t *testing.T) {
	content := `# Tracker

## Work

- [ ] Active task

## Done

- [x] Completed task one
- [x] Completed task two
`
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	if items[0].Done {
		t.Error("first item should not be done")
	}
	if !items[1].Done || !items[2].Done {
		t.Error("done items should be marked done")
	}
}

func TestParseTrackerGoalAtZero(t *testing.T) {
	content := "## Fitness\n\n- [ ] Do 100 pushups [goal: 0/100 pushups]\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if items[0].Current != 0 {
		t.Errorf("current: got %v, want 0", items[0].Current)
	}
	if items[0].Target != 100 {
		t.Errorf("target: got %v, want 100", items[0].Target)
	}
}

func TestParseTrackerGoalAt100Percent(t *testing.T) {
	content := "## Fitness\n\n- [ ] Do 100 pushups [goal: 100/100 pushups]\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if items[0].Current != 100 || items[0].Target != 100 {
		t.Errorf("expected 100/100, got %v/%v", items[0].Current, items[0].Target)
	}
}

func TestParseTrackerGoalOver100Percent(t *testing.T) {
	content := "## Fitness\n\n- [ ] Do 100 pushups [goal: 120/100 pushups]\n"
	path := filepath.Join(t.TempDir(), "tracker.md")
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if items[0].Current != 120 {
		t.Errorf("current: got %v, want 120", items[0].Current)
	}
}
