package test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

func TestParseIdeas_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	content := `# Ideas

- [ ] Try Caddy instead of nginx [status: parked] [tags: infra, homelab] [project: homelabs] [added: 2026-03-14]
  Replace nginx reverse proxy with Caddy for automatic HTTPS.

- [ ] Dashboard mobile PWA [status: untriaged] [tags: dashboard] [added: 2026-03-16] [images: pwa-sketch.png]
  Add a manifest.json and service worker.
`
	os.WriteFile(path, []byte(content), 0o644)

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 ideas, got %d", len(parsed))
	}

	idea := parsed[0]
	if idea.Title != "Try Caddy instead of nginx" {
		t.Errorf("title: got %q", idea.Title)
	}
	if idea.Status != "parked" {
		t.Errorf("status: got %q", idea.Status)
	}
	if !slices.Equal(idea.Tags, []string{"infra", "homelab"}) {
		t.Errorf("tags: got %v", idea.Tags)
	}
	if idea.Project != "homelabs" {
		t.Errorf("project: got %q", idea.Project)
	}
	if idea.Added != "2026-03-14" {
		t.Errorf("added: got %q", idea.Added)
	}
	if idea.Body != "Replace nginx reverse proxy with Caddy for automatic HTTPS." {
		t.Errorf("body: got %q", idea.Body)
	}

	idea2 := parsed[1]
	if idea2.Status != "untriaged" {
		t.Errorf("idea2 status: got %q", idea2.Status)
	}
	if !slices.Equal(idea2.Images, []string{"pwa-sketch.png"}) {
		t.Errorf("idea2 images: got %v", idea2.Images)
	}
}

func TestParseIdeas_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 0 {
		t.Fatalf("expected 0 ideas, got %d", len(parsed))
	}
}

func TestParseIdeas_NonExistent(t *testing.T) {
	parsed, err := ideas.ParseIdeas("/nonexistent/ideas.md")
	if err != nil {
		t.Fatalf("should not error on missing file: %v", err)
	}
	if parsed != nil {
		t.Fatalf("expected nil, got %v", parsed)
	}
}

func TestParseIdeas_DefaultStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	content := "# Ideas\n\n- [ ] No status idea [added: 2026-03-16]\n"
	os.WriteFile(path, []byte(content), 0o644)

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(parsed))
	}
	if parsed[0].Status != "untriaged" {
		t.Errorf("default status should be untriaged, got %q", parsed[0].Status)
	}
}

func TestRoundTrip_PreservesBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			Slug:    "test-idea",
			Title:   "Test Idea",
			Status:  "untriaged",
			Tags:    []string{"go", "testing"},
			Project: "dashboard",
			Added:   "2026-03-16",
			Images:  []string{"img1.png"},
			Body:    "Paragraph one.\n\nParagraph two.\n\nParagraph three.",
		},
	}

	if err := ideas.WriteIdeas(path, "Ideas", original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(parsed))
	}

	got := parsed[0]
	if got.Title != original[0].Title {
		t.Errorf("title: got %q, want %q", got.Title, original[0].Title)
	}
	if got.Status != original[0].Status {
		t.Errorf("status: got %q, want %q", got.Status, original[0].Status)
	}
	if !slices.Equal(got.Tags, original[0].Tags) {
		t.Errorf("tags: got %v, want %v", got.Tags, original[0].Tags)
	}
	if got.Project != original[0].Project {
		t.Errorf("project: got %q, want %q", got.Project, original[0].Project)
	}
	if got.Added != original[0].Added {
		t.Errorf("added: got %q, want %q", got.Added, original[0].Added)
	}
	if !slices.Equal(got.Images, original[0].Images) {
		t.Errorf("images: got %v, want %v", got.Images, original[0].Images)
	}
	if got.Body != original[0].Body {
		t.Errorf("body: got %q, want %q", got.Body, original[0].Body)
	}
}

func TestRoundTrip_MultipleIdeas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{Slug: "first", Title: "First", Status: "untriaged", Added: "2026-03-14", Body: "Body one."},
		{Slug: "second", Title: "Second", Status: "parked", Added: "2026-03-15", Body: "Body two."},
		{Slug: "third", Title: "Third", Status: "dropped", Added: "2026-03-16"},
	}

	if err := ideas.WriteIdeas(path, "Ideas", original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 3 {
		t.Fatalf("expected 3 ideas, got %d", len(parsed))
	}

	for i, want := range original {
		got := parsed[i]
		if got.Title != want.Title {
			t.Errorf("idea %d title: got %q, want %q", i, got.Title, want.Title)
		}
		if got.Status != want.Status {
			t.Errorf("idea %d status: got %q, want %q", i, got.Status, want.Status)
		}
		if got.Body != want.Body {
			t.Errorf("idea %d body: got %q, want %q", i, got.Body, want.Body)
		}
	}
}

func TestServiceCRUD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)

	// Add.
	idea := &ideas.Idea{
		Slug:   "my-idea",
		Title:  "My Idea",
		Tags:   []string{"test"},
		Body:   "Some content.",
	}
	if err := svc.Add(idea); err != nil {
		t.Fatalf("add: %v", err)
	}

	// List.
	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].Title != "My Idea" {
		t.Errorf("title: got %q", list[0].Title)
	}
	if list[0].Status != "untriaged" {
		t.Errorf("status should default to untriaged, got %q", list[0].Status)
	}

	// Get.
	got, err := svc.Get("my-idea")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "My Idea" {
		t.Errorf("get title: got %q", got.Title)
	}

	// Edit.
	if err := svc.Edit("my-idea", "", "Updated content.", []string{"test", "updated"}, nil); err != nil {
		t.Fatalf("edit: %v", err)
	}
	updated, _ := svc.Get("my-idea")
	if !slices.Equal(updated.Tags, []string{"test", "updated"}) {
		t.Errorf("updated tags: got %v", updated.Tags)
	}
	if updated.Body != "Updated content." {
		t.Errorf("updated body: got %q", updated.Body)
	}

	// Delete (soft-delete).
	if err := svc.Delete("my-idea"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 0 {
		t.Errorf("expected 0 in List after soft delete, got %d", len(list))
	}

	// Soft-deleted item still accessible via Get.
	deleted, err := svc.Get("my-idea")
	if err != nil {
		t.Fatalf("get soft-deleted: %v", err)
	}
	if deleted.DeletedAt == "" {
		t.Error("expected DeletedAt to be set after soft delete")
	}

	// Permanent delete removes completely.
	if err := svc.Restore("my-idea"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 1 {
		t.Errorf("expected 1 after restore, got %d", len(list))
	}

	if err := svc.PermanentDelete("my-idea"); err != nil {
		t.Fatalf("permanent delete: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 0 {
		t.Errorf("expected 0 after permanent delete, got %d", len(list))
	}
}

func TestServiceTriage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{
		Slug:  "park-me",
		Title: "Park Me",
		Body:  "To be parked.",
	})

	if err := svc.Triage("park-me", "park"); err != nil {
		t.Fatalf("triage park: %v", err)
	}

	idea, err := svc.Get("park-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if idea.Status != "parked" {
		t.Errorf("status: got %q, want parked", idea.Status)
	}

	if err := svc.Triage("park-me", "drop"); err != nil {
		t.Fatalf("triage drop: %v", err)
	}
	idea, _ = svc.Get("park-me")
	if idea.Status != "dropped" {
		t.Errorf("status: got %q, want dropped", idea.Status)
	}

	if err := svc.Triage("park-me", "untriage"); err != nil {
		t.Fatalf("triage untriage: %v", err)
	}
	idea, _ = svc.Get("park-me")
	if idea.Status != "untriaged" {
		t.Errorf("status: got %q, want untriaged", idea.Status)
	}
}

func TestConvertedToRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			Slug:        "converted-idea",
			Title:       "Converted Idea",
			Status:      "converted",
			Tags:        []string{"feature"},
			Added:       "2026-03-16",
			ConvertedTo: "converted-idea",
			Body:        "This was converted to a task.",
		},
		{
			Slug:   "normal-idea",
			Title:  "Normal Idea",
			Status: "untriaged",
			Added:  "2026-03-17",
			Body:   "Still an idea.",
		},
	}

	if err := ideas.WriteIdeas(path, "Ideas", original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 ideas, got %d", len(parsed))
	}

	got := parsed[0]
	if got.Status != "converted" {
		t.Errorf("status: got %q, want %q", got.Status, "converted")
	}
	if got.ConvertedTo != "converted-idea" {
		t.Errorf("converted-to: got %q, want %q", got.ConvertedTo, "converted-idea")
	}

	normal := parsed[1]
	if normal.ConvertedTo != "" {
		t.Errorf("expected empty converted-to, got %q", normal.ConvertedTo)
	}
}

func TestConvertedToPreservesBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			Slug:        "rich-idea",
			Title:       "Rich Idea",
			Status:      "converted",
			ConvertedTo: "rich-task",
			Added:       "2026-03-16",
			Body:        "Paragraph one.\n\nParagraph two.\n\nParagraph three.",
		},
	}

	if err := ideas.WriteIdeas(path, "Ideas", original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(parsed))
	}
	if parsed[0].Body != original[0].Body {
		t.Errorf("body: got %q, want %q", parsed[0].Body, original[0].Body)
	}
}

func TestServiceMarkConverted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{
		Slug:  "convert-me",
		Title: "Convert Me",
		Body:  "To be converted.",
	})

	if err := svc.MarkConverted("convert-me", "convert-me"); err != nil {
		t.Fatalf("mark converted: %v", err)
	}

	idea, err := svc.Get("convert-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if idea.Status != "converted" {
		t.Errorf("status: got %q, want converted", idea.Status)
	}
	if idea.ConvertedTo != "convert-me" {
		t.Errorf("converted-to: got %q, want %q", idea.ConvertedTo, "convert-me")
	}

	// Verify the idea is not deleted.
	list, _ := svc.List()
	if len(list) != 1 {
		t.Errorf("expected 1 idea after conversion, got %d (idea should NOT be deleted)", len(list))
	}
}

func TestConversionFlowWithLinkage(t *testing.T) {
	dir := t.TempDir()
	ideasPath := filepath.Join(dir, "ideas.md")
	trackerPath := filepath.Join(dir, "tracker.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644)
	os.WriteFile(trackerPath, []byte("# Personal\n\n"), 0o644)

	ideaSvc := ideas.NewService(ideasPath)
	ideaSvc.Add(&ideas.Idea{
		Slug:  "my-feature",
		Title: "My Feature",
		Tags:  []string{"tech"},
		Body:  "Build a new feature.",
	})

	// Simulate the full conversion flow.
	idea, _ := ideaSvc.Get("my-feature")

	// Create tracker item with FromIdea set.
	taskItem := tracker.Item{
		Title:    idea.Title,
		Type:     tracker.TaskType,
		Body:     idea.Body,
		Tags:     idea.Tags,
		FromIdea: idea.Slug,
	}
	items := []tracker.Item{taskItem}
	if err := tracker.WriteTracker(trackerPath, "Personal", items); err != nil {
		t.Fatalf("write tracker: %v", err)
	}

	// Mark idea as converted.
	taskSlug := tracker.Slugify(idea.Title)
	if err := ideaSvc.MarkConverted("my-feature", taskSlug); err != nil {
		t.Fatalf("mark converted: %v", err)
	}

	// Verify linkage on both sides.
	converted, _ := ideaSvc.Get("my-feature")
	if converted.Status != "converted" {
		t.Errorf("idea status: got %q, want converted", converted.Status)
	}
	if converted.ConvertedTo != taskSlug {
		t.Errorf("idea converted-to: got %q, want %q", converted.ConvertedTo, taskSlug)
	}

	parsedItems, _ := tracker.ParseTracker(trackerPath)
	if len(parsedItems) != 1 {
		t.Fatalf("expected 1 task, got %d", len(parsedItems))
	}
	if parsedItems[0].FromIdea != "my-feature" {
		t.Errorf("task from-idea: got %q, want %q", parsedItems[0].FromIdea, "my-feature")
	}
}

func TestServiceAddResearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{
		Slug:  "research-me",
		Title: "Research Me",
		Body:  "Initial content.",
	})

	if err := svc.AddResearch("research-me", "Some research findings."); err != nil {
		t.Fatalf("add research: %v", err)
	}

	idea, _ := svc.Get("research-me")
	if !strings.Contains(idea.Body, "## Research") {
		t.Errorf("body should contain ## Research heading, got %q", idea.Body)
	}
	if !strings.Contains(idea.Body, "Some research findings.") {
		t.Errorf("body should contain research content, got %q", idea.Body)
	}
	if !strings.Contains(idea.Body, "Initial content.") {
		t.Errorf("body should still contain initial content, got %q", idea.Body)
	}
}

