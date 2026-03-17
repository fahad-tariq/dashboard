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

	err := svc.Edit("original-title", "# New Title\n\nSome body text.", nil, nil)
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

	err := svc.Edit("keep-slug", "Updated body content.", nil, nil)
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

	err := svc.Edit("beta", "# Alpha\n\nNew body for beta.", nil, nil)
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

	err := svc.Edit("has-title", "# \n\nBody without title.", nil, nil)

	// The service permits blank titles from headings (no validation).
	// Verify the idea still exists regardless of slug change.
	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 idea after edit, got %d (edit err: %v)", len(list), err)
	}
}

func TestIdeasServiceEdit_NonExistentSlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)

	svc := ideas.NewService(path)
	svc.Add(&ideas.Idea{Slug: "exists", Title: "Exists", Body: "Here."})

	err := svc.Edit("does-not-exist", "New body.", nil, nil)
	if err == nil {
		t.Fatal("expected error editing non-existent slug, got nil")
	}
}
