package test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

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
			ID:      "tstd1231",
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
		{ID: "frst1231", Title: "First", Status: "untriaged", Added: "2026-03-14", Body: "Body one."},
		{ID: "scnd1231", Title: "Second", Status: "parked", Added: "2026-03-15", Body: "Body two."},
		{ID: "thrd1231", Title: "Third", Status: "dropped", Added: "2026-03-16"},
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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)

	// Add.
	idea := &ideas.Idea{
		ID:    "md123451",
		Title: "My Idea",
		Tags:  []string{"test"},
		Body:  "Some content.",
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
	got, err := svc.Get("md123451")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "My Idea" {
		t.Errorf("get title: got %q", got.Title)
	}

	// Edit.
	if err := svc.Edit("md123451", "", "Updated content.", []string{"test", "updated"}, nil); err != nil {
		t.Fatalf("edit: %v", err)
	}
	updated, _ := svc.Get("md123451")
	if !slices.Equal(updated.Tags, []string{"test", "updated"}) {
		t.Errorf("updated tags: got %v", updated.Tags)
	}
	if updated.Body != "Updated content." {
		t.Errorf("updated body: got %q", updated.Body)
	}

	// Delete (soft-delete).
	if err := svc.Delete("md123451"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 0 {
		t.Errorf("expected 0 in List after soft delete, got %d", len(list))
	}

	// Soft-deleted item still accessible via Get.
	deleted, err := svc.Get("md123451")
	if err != nil {
		t.Fatalf("get soft-deleted: %v", err)
	}
	if deleted.DeletedAt == "" {
		t.Error("expected DeletedAt to be set after soft delete")
	}

	// Permanent delete removes completely.
	if err := svc.Restore("md123451"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 1 {
		t.Errorf("expected 1 after restore, got %d", len(list))
	}

	if err := svc.PermanentDelete("md123451"); err != nil {
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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{
		ID:    "prkm1231",
		Title: "Park Me",
		Body:  "To be parked.",
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.Triage("prkm1231", "park"); err != nil {
		t.Fatalf("triage park: %v", err)
	}

	idea, err := svc.Get("prkm1231")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if idea.Status != "parked" {
		t.Errorf("status: got %q, want parked", idea.Status)
	}

	if err := svc.Triage("prkm1231", "drop"); err != nil {
		t.Fatalf("triage drop: %v", err)
	}
	idea, _ = svc.Get("prkm1231")
	if idea.Status != "dropped" {
		t.Errorf("status: got %q, want dropped", idea.Status)
	}

	if err := svc.Triage("prkm1231", "untriage"); err != nil {
		t.Fatalf("triage untriage: %v", err)
	}
	idea, _ = svc.Get("prkm1231")
	if idea.Status != "untriaged" {
		t.Errorf("status: got %q, want untriaged", idea.Status)
	}
}

func TestConvertedToRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			ID:          "cnvrtdd1",
			Title:       "Converted Idea",
			Status:      "converted",
			Tags:        []string{"feature"},
			Added:       "2026-03-16",
			ConvertedTo: "cnvrtdd1",
			Body:        "This was converted to a task.",
		},
		{
			ID:     "nrmld121",
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
	if got.ConvertedTo != "cnvrtdd1" {
		t.Errorf("converted-to: got %q, want %q", got.ConvertedTo, "cnvrtdd1")
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
			ID:          "rchd1231",
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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{
		ID:    "cnvrtm11",
		Title: "Convert Me",
		Body:  "To be converted.",
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkConverted("cnvrtm11", "cnvrtm11"); err != nil {
		t.Fatalf("mark converted: %v", err)
	}

	idea, err := svc.Get("cnvrtm11")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if idea.Status != "converted" {
		t.Errorf("status: got %q, want converted", idea.Status)
	}
	if idea.ConvertedTo != "cnvrtm11" {
		t.Errorf("converted-to: got %q, want %q", idea.ConvertedTo, "cnvrtm11")
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
	if err := os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trackerPath, []byte("# Personal\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ideaSvc := ideas.NewService(ideasPath, time.UTC)
	if err := ideaSvc.Add(&ideas.Idea{
		ID:    "mftr1231",
		Title: "My Feature",
		Tags:  []string{"tech"},
		Body:  "Build a new feature.",
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate the full conversion flow.
	idea, _ := ideaSvc.Get("mftr1231")

	// Create tracker item with FromIdea set.
	taskItem := tracker.Item{
		ID:       "b7k2m9xq",
		Title:    idea.Title,
		Type:     tracker.TaskType,
		Body:     idea.Body,
		Tags:     idea.Tags,
		FromIdea: idea.ID,
	}
	items := []tracker.Item{taskItem}
	if err := tracker.WriteTracker(trackerPath, "Personal", items); err != nil {
		t.Fatalf("write tracker: %v", err)
	}

	// Mark idea as converted.
	taskID := taskItem.ID
	if err := ideaSvc.MarkConverted("mftr1231", taskID); err != nil {
		t.Fatalf("mark converted: %v", err)
	}

	// Verify linkage on both sides.
	converted, _ := ideaSvc.Get("mftr1231")
	if converted.Status != "converted" {
		t.Errorf("idea status: got %q, want converted", converted.Status)
	}
	if converted.ConvertedTo != taskID {
		t.Errorf("idea converted-to: got %q, want %q", converted.ConvertedTo, taskID)
	}

	parsedItems, _ := tracker.ParseTracker(trackerPath)
	if len(parsedItems) != 1 {
		t.Fatalf("expected 1 task, got %d", len(parsedItems))
	}
	if parsedItems[0].FromIdea != "mftr1231" {
		t.Errorf("task from-idea: got %q, want %q", parsedItems[0].FromIdea, "mftr1231")
	}
}

func TestIdeasCaptionedImagesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			ID:     "cptndd11",
			Title:  "Captioned Idea",
			Status: "untriaged",
			Added:  "2026-03-16",
			Images: []string{"photo.png|A nice photo", "diagram.jpg"},
			Body:   "Some body text.",
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
	if !slices.Equal(parsed[0].Images, []string{"photo.png|A nice photo", "diagram.jpg"}) {
		t.Errorf("images: got %v, want [photo.png|A nice photo diagram.jpg]", parsed[0].Images)
	}
}

func TestServiceAddResearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{
		ID:    "rsrchm11",
		Title: "Research Me",
		Body:  "Initial content.",
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.AddResearch("rsrchm11", "Some research findings."); err != nil {
		t.Fatalf("add research: %v", err)
	}

	idea, _ := svc.Get("rsrchm11")
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
