package test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fahad/dashboard/internal/exploration"
)

func TestParseExploration(t *testing.T) {
	dir := t.TempDir()
	content := `---
tags: rust, systems
date: 2026-03-16
---

# Exploring Rust for CLI tools

Some initial thoughts on using Rust.
`
	path := filepath.Join(dir, "exploring-rust-for-cli-tools.md")
	os.WriteFile(path, []byte(content), 0o644)

	e, err := exploration.ParseExploration(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if e.Slug != "exploring-rust-for-cli-tools" {
		t.Errorf("slug: got %q", e.Slug)
	}
	if e.Title != "Exploring Rust for CLI tools" {
		t.Errorf("title: got %q", e.Title)
	}
	if !slices.Equal(e.Tags, []string{"rust", "systems"}) {
		t.Errorf("tags: got %v", e.Tags)
	}
	if e.Date != "2026-03-16" {
		t.Errorf("date: got %q", e.Date)
	}
}

func TestExplorationRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &exploration.Exploration{
		Slug:   "test-exploration",
		Title:  "Test Exploration",
		Tags:   []string{"go", "testing"},
		Date:   "2026-03-16",
		Images: []string{"img1.png"},
		Body:   "# Test Exploration\n\nSome body text.",
	}

	if err := exploration.WriteExploration(dir, original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := exploration.ParseExploration(filepath.Join(dir, "test-exploration.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !slices.Equal(parsed.Tags, original.Tags) {
		t.Errorf("tags: got %v, want %v", parsed.Tags, original.Tags)
	}
	if parsed.Title != original.Title {
		t.Errorf("title: got %q", parsed.Title)
	}
	if !slices.Equal(parsed.Images, original.Images) {
		t.Errorf("images: got %v, want %v", parsed.Images, original.Images)
	}
}

func TestExplorationQuickAdd(t *testing.T) {
	tests := []struct {
		input string
		title string
		tags  []string
	}{
		{"Exploring WASM #rust #web", "Exploring WASM", []string{"rust", "web"}},
		{"Simple exploration", "Simple exploration", nil},
		{"#tagged #only", "", []string{"tagged", "only"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			title, tags := exploration.ParseQuickAdd(tt.input)
			if title != tt.title {
				t.Errorf("title: got %q, want %q", title, tt.title)
			}
			if !slices.Equal(tags, tt.tags) {
				t.Errorf("tags: got %v, want %v", tags, tt.tags)
			}
		})
	}
}

func TestExplorationService_CRUD(t *testing.T) {
	dir := t.TempDir()
	svc := exploration.NewService(dir)

	// Add.
	e := &exploration.Exploration{
		Slug:  "my-exploration",
		Title: "My Exploration",
		Tags:  []string{"test"},
		Body:  "# My Exploration\n\nInitial content.",
	}
	if err := svc.Add(e); err != nil {
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
	if list[0].Title != "My Exploration" {
		t.Errorf("title: got %q", list[0].Title)
	}

	// Get.
	got, err := svc.Get("my-exploration")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "My Exploration" {
		t.Errorf("get title: got %q", got.Title)
	}

	// Update.
	if err := svc.Update("my-exploration", "# My Exploration\n\nUpdated.", []string{"test", "updated"}, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, _ := svc.Get("my-exploration")
	if !slices.Equal(updated.Tags, []string{"test", "updated"}) {
		t.Errorf("updated tags: got %v", updated.Tags)
	}

	// Delete.
	if err := svc.Delete("my-exploration"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list))
	}
}
