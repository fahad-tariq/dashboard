package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/ideas"
)

func TestIdeasServiceEdit_TitleOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{
		ID:    "rgnlttl1",
		Title: "Original Title",
		Body:  "Some body text.",
	}); err != nil {
		t.Fatal(err)
	}

	err := svc.Edit("rgnlttl1", "", "# New Title\n\nSome body text.", nil, nil)
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
	if list[0].ID != "rgnlttl1" {
		t.Errorf("ID should stay %q, got %q", "rgnlttl1", list[0].ID)
	}

	got, err := svc.Get("rgnlttl1")
	if err != nil {
		t.Fatalf("get after rename: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("get title: got %q", got.Title)
	}
}

func TestIdeasServiceEdit_BodyOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{
		ID:    "kpslg121",
		Title: "Keep Slug",
		Body:  "Old body.",
	}); err != nil {
		t.Fatal(err)
	}

	err := svc.Edit("kpslg121", "", "Updated body content.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	got, err := svc.Get("kpslg121")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "kpslg121" {
		t.Errorf("ID should remain %q, got %q", "kpslg121", got.ID)
	}
	if got.Body != "Updated body content." {
		t.Errorf("body: got %q", got.Body)
	}
	if got.Title != "Keep Slug" {
		t.Errorf("title should remain %q, got %q", "Keep Slug", got.Title)
	}
}

func TestIdeasServiceEdit_DuplicateTitles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha", Body: "First."}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(&ideas.Idea{ID: "bt123451", Title: "Beta", Body: "Second."}); err != nil {
		t.Fatal(err)
	}

	err := svc.Edit("bt123451", "", "# Alpha\n\nNew body for beta.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	list, _ := svc.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 ideas, got %d", len(list))
	}

	// Both ideas are titled Alpha and keep their own IDs.
	for _, idea := range list {
		if idea.Title != "Alpha" {
			t.Errorf("expected both titles to be Alpha, got %q", idea.Title)
		}
	}
	if list[0].ID != "lph12341" || list[1].ID != "bt123451" {
		t.Errorf("IDs = %q, %q; want lph12341, bt123451", list[0].ID, list[1].ID)
	}
}

func TestIdeasServiceEdit_BlankTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "hsttl121", Title: "Has Title", Body: "Content."}); err != nil {
		t.Fatal(err)
	}

	err := svc.Edit("hsttl121", "", "# \n\nBody without title.", nil, nil)

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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "ldd12341", Title: "Old Idea", Body: "Body."}); err != nil {
		t.Fatal(err)
	}

	err := svc.Edit("ldd12341", "Renamed Idea", "Body.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	// The ID survives the rename.
	got, err := svc.Get("ldd12341")
	if err != nil {
		t.Fatalf("get after rename: %v", err)
	}
	if got.Title != "Renamed Idea" {
		t.Errorf("title: got %q, want %q", got.Title, "Renamed Idea")
	}
}

func TestIdeasServiceEdit_ExplicitTitleOverridesBodyHeading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "tst12341", Title: "Test", Body: "Body."}); err != nil {
		t.Fatal(err)
	}

	// Explicit title should win over body heading.
	err := svc.Edit("tst12341", "Explicit Title", "# Body Heading\n\nContent.", nil, nil)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	got, err := svc.Get("tst12341")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Explicit Title" {
		t.Errorf("title: got %q, want %q", got.Title, "Explicit Title")
	}
}

func TestIdeasServiceEdit_NonExistentID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "xsts1231", Title: "Exists", Body: "Here."}); err != nil {
		t.Fatal(err)
	}

	err := svc.Edit("does-not-exist", "", "New body.", nil, nil)
	if err == nil {
		t.Fatal("expected error editing non-existent slug, got nil")
	}
}

func TestIdeasServiceSoftDeleteAndRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha", Body: "First."}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(&ideas.Idea{ID: "bt123451", Title: "Beta", Body: "Second."}); err != nil {
		t.Fatal(err)
	}

	// Soft delete Alpha.
	if err := svc.Delete("lph12341"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Alpha excluded from List.
	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 in List, got %d", len(list))
	}

	// Alpha in ListDeleted.
	deleted := svc.ListDeleted()
	if len(deleted) != 1 || deleted[0].ID != "lph12341" {
		t.Fatalf("expected alpha in ListDeleted, got %v", deleted)
	}

	// Alpha excluded from Search.
	results := svc.Search("alpha")
	if len(results) != 0 {
		t.Errorf("soft-deleted idea should not appear in Search, got %d results", len(results))
	}

	// Restore Alpha.
	if err := svc.Restore("lph12341"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	list, _ = svc.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 in List after restore, got %d", len(list))
	}

	restored, _ := svc.Get("lph12341")
	if restored.DeletedAt != "" {
		t.Error("expected DeletedAt to be cleared after restore")
	}
}

func TestIdeasServicePermanentDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha", Body: "First."}); err != nil {
		t.Fatal(err)
	}

	if err := svc.PermanentDelete("lph12341"); err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}

	_, err := svc.Get("lph12341")
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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := ideas.NewService(path, time.UTC)

	if _, err := svc.PurgeExpired(7); err != nil {
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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := ideas.NewService(path, time.UTC)

	if _, err := svc.PurgeExpired(7); err != nil {
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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha", Body: "First."}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(&ideas.Idea{ID: "bt123451", Title: "Beta", Body: "Second."}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(&ideas.Idea{ID: "gmm12341", Title: "Gamma", Body: "Third."}); err != nil {
		t.Fatal(err)
	}

	if err := svc.BulkDelete([]string{"lph12341", "gmm12341"}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 active idea, got %d", len(list))
	}
	if list[0].ID != "bt123451" {
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
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha", Body: "First."}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(&ideas.Idea{ID: "bt123451", Title: "Beta", Body: "Second."}); err != nil {
		t.Fatal(err)
	}

	if err := svc.BulkTriage([]string{"lph12341", "bt123451"}, "park"); err != nil {
		t.Fatalf("BulkTriage: %v", err)
	}

	alpha, _ := svc.Get("lph12341")
	if alpha.Status != "parked" {
		t.Errorf("Alpha status: got %q, want %q", alpha.Status, "parked")
	}
	beta, _ := svc.Get("bt123451")
	if beta.Status != "parked" {
		t.Errorf("Beta status: got %q, want %q", beta.Status, "parked")
	}
}

func TestIdeasServiceBulkTriageDrop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha"}); err != nil {
		t.Fatal(err)
	}

	if err := svc.BulkTriage([]string{"lph12341"}, "drop"); err != nil {
		t.Fatalf("BulkTriage drop: %v", err)
	}

	alpha, _ := svc.Get("lph12341")
	if alpha.Status != "dropped" {
		t.Errorf("Alpha status: got %q, want %q", alpha.Status, "dropped")
	}
}

func TestIdeasServiceBulkTriageInvalidAction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha"}); err != nil {
		t.Fatal(err)
	}

	err := svc.BulkTriage([]string{"lph12341"}, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid triage action")
	}
}

func TestIdeasServiceBulkInvalidSlugRollsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(path, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := ideas.NewService(path, time.UTC)
	if err := svc.Add(&ideas.Idea{ID: "lph12341", Title: "Alpha"}); err != nil {
		t.Fatal(err)
	}

	err := svc.BulkDelete([]string{"lph12341", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}

	// Alpha should NOT be deleted because the batch failed atomically.
	alpha, _ := svc.Get("lph12341")
	if alpha.DeletedAt != "" {
		t.Error("Alpha should not be deleted -- batch should have failed atomically")
	}
}

func TestIdeasDeletedAtRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")

	original := []ideas.Idea{
		{
			ID:        "dltdd121",
			Title:     "Deleted Idea",
			Status:    "untriaged",
			Added:     "2026-03-16",
			DeletedAt: "2026-03-17",
			Body:      "Paragraph one.\n\nParagraph two.",
		},
		{
			ID:     "nrmld121",
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
