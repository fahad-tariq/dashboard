package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/ideas"
)

func TestIdeasServiceEdit_TitleOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{
		Slug:  "original-title",
		Title: "Original Title",
		Body:  "Some body text.",
	})

	err := svc.Edit("original-title", "", "# New Title\n\nSome body text.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(list))
	}
	if list[0].Title != "New Title" {
		t.Errorf("title should be %q, got %q", "New Title", list[0].Title)
	}
	if list[0].Slug != ideas.Slugify("New Title") {
		t.Errorf("slug should update to %q, got %q", ideas.Slugify("New Title"), list[0].Slug)
	}

	_, err = svc.Get("original-title")
	if err == nil {
		t.Error("old slug should no longer resolve")
	}

	got, err := svc.Get(list[0].Slug)
	if err != nil {
		t.Fatalf("get by new slug: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("get title: got %q", got.Title)
	}
}

func TestIdeasServiceEdit_BodyOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{
		Slug:  "keep-slug",
		Title: "Keep Slug",
		Body:  "Old body.",
	})

	err := svc.Edit("keep-slug", "", "Updated body content.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	got, err := svc.Get("keep-slug")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Slug != "keep-slug" {
		t.Errorf("slug should remain %q, got %q", "keep-slug", got.Slug)
	}
	if got.Body != "Updated body content." {
		t.Errorf("body: got %q", got.Body)
	}
	if got.Title != "Keep Slug" {
		t.Errorf("title should remain %q, got %q", "Keep Slug", got.Title)
	}
}

func TestIdeasServiceEdit_TitleCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha", Body: "First."})
	svc.Add(&ideas.Idea{Slug: "beta", Title: "Beta", Body: "Second."})

	err := svc.Edit("beta", "", "# Alpha\n\nNew body for beta.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	list, _ := svc.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 ideas, got %d", len(list))
	}

	// Service does not deduplicate slugs on collision -- both ideas end up
	// with the same slug. Get returns whichever appears first.
	for _, idea := range list {
		if idea.Slug != "alpha" {
			t.Errorf("expected both slugs to be %q, got %q", "alpha", idea.Slug)
		}
	}
}

func TestIdeasServiceEdit_BlankTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "has-title", Title: "Has Title", Body: "Content."})

	err := svc.Edit("has-title", "", "# \n\nBody without title.", nil, nil)

	// The service permits blank titles from headings (no validation).
	// Verify the idea still exists regardless of slug change.
	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 idea after edit, got %d (edit err: %v)", len(list), err)
	}
}

func TestIdeasServiceEdit_ExplicitTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "old-idea", Title: "Old Idea", Body: "Body."})

	err := svc.Edit("old-idea", "Renamed Idea", "Body.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	// Old slug gone.
	_, err = svc.Get("old-idea")
	if err == nil {
		t.Error("old slug should no longer resolve")
	}

	// New slug exists with updated title.
	got, err := svc.Get("renamed-idea")
	if err != nil {
		t.Fatalf("get by new slug: %v", err)
	}
	if got.Title != "Renamed Idea" {
		t.Errorf("title: got %q, want %q", got.Title, "Renamed Idea")
	}
}

func TestIdeasServiceEdit_ExplicitTitleOverridesBodyHeading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "test", Title: "Test", Body: "Body."})

	// Explicit title should win over body heading.
	err := svc.Edit("test", "Explicit Title", "# Body Heading\n\nContent.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	got, err := svc.Get("explicit-title")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Explicit Title" {
		t.Errorf("title: got %q, want %q", got.Title, "Explicit Title")
	}
}

func TestIdeasServiceEdit_NonExistentSlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "exists", Title: "Exists", Body: "Here."})

	err := svc.Edit("does-not-exist", "", "New body.", nil, nil)
	if err == nil {
		t.Fatal("expected error editing non-existent slug, got nil")
	}
}

func TestIdeasServiceSoftDeleteAndRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha", Body: "First."})
	svc.Add(&ideas.Idea{Slug: "beta", Title: "Beta", Body: "Second."})

	// Soft delete Alpha.
	if err := svc.Delete("alpha"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Alpha excluded from List.
	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 in List, got %d", len(list))
	}

	// Alpha in ListDeleted.
	deleted := svc.ListDeleted()
	if len(deleted) != 1 || deleted[0].Slug != "alpha" {
		t.Fatalf("expected alpha in ListDeleted, got %v", deleted)
	}

	// Alpha excluded from Search.
	results := svc.Search("alpha")
	if len(results) != 0 {
		t.Errorf("soft-deleted idea should not appear in Search, got %d results", len(results))
	}

	// Restore Alpha.
	if err := svc.Restore("alpha"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	list, _ = svc.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 in List after restore, got %d", len(list))
	}

	restored, _ := svc.Get("alpha")
	if restored.DeletedAt != "" {
		t.Error("expected DeletedAt to be cleared after restore")
	}
}

func TestIdeasServicePermanentDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha", Body: "First."})

	if err := svc.PermanentDelete("alpha"); err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}

	_, err := svc.Get("alpha")
	if err == nil {
		t.Error("expected error after permanent delete")
	}
}

func TestIdeasServicePurgeExpired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	// Write ideas with deleted dates directly.
	content := `# Ideas

- [ ] Active idea [status: untriaged] [added: 2026-03-01]
  Active body.

- [ ] Old trash [status: untriaged] [added: 2026-03-01] [deleted: 2020-01-01]
  Old body.

- [ ] Recent trash [status: untriaged] [added: 2026-03-01] [deleted: 2099-12-31]
  Recent body.
`
	os.WriteFile(path, []byte(content), 0o644)
	svc := ideas.NewService(path)

	if err := svc.PurgeExpired(7); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}

	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 active idea, got %d", len(list))
	}
	if list[0].Title != "Active idea" {
		t.Errorf("expected Active idea, got %q", list[0].Title)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 1 || deleted[0].Title != "Recent trash" {
		t.Errorf("expected only Recent trash in deleted, got %v", deleted)
	}
}

func TestIdeasServicePurgeExpiredMalformedDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	// Use a date that matches the regex pattern but is invalid for time.Parse.
	content := "# Ideas\n\n- [ ] Bad date [status: untriaged] [deleted: 2026-13-45]\n"
	os.WriteFile(path, []byte(content), 0o644)
	svc := ideas.NewService(path)

	if err := svc.PurgeExpired(7); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 1 {
		t.Errorf("expected malformed-date item to be kept, got %d", len(deleted))
	}
}

func TestIdeasServiceBulkDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha", Body: "First."})
	svc.Add(&ideas.Idea{Slug: "beta", Title: "Beta", Body: "Second."})
	svc.Add(&ideas.Idea{Slug: "gamma", Title: "Gamma", Body: "Third."})

	if err := svc.BulkDelete([]string{"alpha", "gamma"}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 active idea, got %d", len(list))
	}
	if list[0].Slug != "beta" {
		t.Errorf("expected Beta, got %q", list[0].Title)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deleted ideas, got %d", len(deleted))
	}
}

func TestIdeasServiceBulkTriage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha", Body: "First."})
	svc.Add(&ideas.Idea{Slug: "beta", Title: "Beta", Body: "Second."})

	if err := svc.BulkTriage([]string{"alpha", "beta"}, "park"); err != nil {
		t.Fatalf("BulkTriage: %v", err)
	}

	alpha, _ := svc.Get("alpha")
	if alpha.Status != "parked" {
		t.Errorf("Alpha status: got %q, want %q", alpha.Status, "parked")
	}
	beta, _ := svc.Get("beta")
	if beta.Status != "parked" {
		t.Errorf("Beta status: got %q, want %q", beta.Status, "parked")
	}
}

func TestIdeasServiceBulkTriageDrop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha"})

	if err := svc.BulkTriage([]string{"alpha"}, "drop"); err != nil {
		t.Fatalf("BulkTriage drop: %v", err)
	}

	alpha, _ := svc.Get("alpha")
	if alpha.Status != "dropped" {
		t.Errorf("Alpha status: got %q, want %q", alpha.Status, "dropped")
	}
}

func TestIdeasServiceBulkTriageInvalidAction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha"})

	err := svc.BulkTriage([]string{"alpha"}, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid triage action")
	}
}

func TestIdeasServiceBulkInvalidSlugRollsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "alpha", Title: "Alpha"})

	err := svc.BulkDelete([]string{"alpha", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}

	// Alpha should NOT be deleted because the batch failed atomically.
	alpha, _ := svc.Get("alpha")
	if alpha.DeletedAt != "" {
		t.Error("Alpha should not be deleted -- batch should have failed atomically")
	}
}

func TestIdeasDeletedAtRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			Slug:      "deleted-idea",
			Title:     "Deleted Idea",
			Status:    "untriaged",
			Added:     "2026-03-16",
			DeletedAt: "2026-03-17",
			Body:      "Paragraph one.\n\nParagraph two.",
		},
		{
			Slug:   "normal-idea",
			Title:  "Normal Idea",
			Status: "untriaged",
			Added:  "2026-03-16",
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

	if parsed[0].DeletedAt != "2026-03-17" {
		t.Errorf("deleted-at: got %q, want %q", parsed[0].DeletedAt, "2026-03-17")
	}
	if parsed[0].Body != "Paragraph one.\n\nParagraph two." {
		t.Errorf("body: got %q, want %q", parsed[0].Body, "Paragraph one.\n\nParagraph two.")
	}

	if parsed[1].DeletedAt != "" {
		t.Errorf("expected empty deleted-at, got %q", parsed[1].DeletedAt)
	}
}
