package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/ideas"
)

func TestParseIdea(t *testing.T) {
	dir := t.TempDir()
	content := `---
type: technical-exploration
suggested-project: homelabs
date: 2026-03-14
research: research/caddy-vs-nginx.md
---

# Try Caddy instead of nginx

Replace nginx reverse proxy with Caddy for automatic HTTPS and simpler config.
`
	path := filepath.Join(dir, "try-caddy-instead-of-nginx.md")
	os.WriteFile(path, []byte(content), 0o644)

	idea, err := ideas.ParseIdea(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if idea.Slug != "try-caddy-instead-of-nginx" {
		t.Errorf("slug: got %q", idea.Slug)
	}
	if idea.Title != "Try Caddy instead of nginx" {
		t.Errorf("title: got %q", idea.Title)
	}
	if idea.Type != "technical-exploration" {
		t.Errorf("type: got %q", idea.Type)
	}
	if idea.SuggestedProject != "homelabs" {
		t.Errorf("suggested-project: got %q", idea.SuggestedProject)
	}
	if idea.Research != "research/caddy-vs-nginx.md" {
		t.Errorf("research: got %q", idea.Research)
	}
}

func TestWriteAndParseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &ideas.Idea{
		Slug:             "test-idea",
		Title:            "Test Idea",
		Type:             "feature",
		SuggestedProject: "dashboard",
		Date:             "2026-03-14",
		Body:             "# Test Idea\n\nSome description here.",
	}

	if err := ideas.WriteIdea(dir, original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := ideas.ParseIdea(filepath.Join(dir, "test-idea.md"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Type != original.Type {
		t.Errorf("type: got %q, want %q", parsed.Type, original.Type)
	}
	if parsed.Title != original.Title {
		t.Errorf("title: got %q, want %q", parsed.Title, original.Title)
	}
	if parsed.SuggestedProject != original.SuggestedProject {
		t.Errorf("suggested-project: got %q, want %q", parsed.SuggestedProject, original.SuggestedProject)
	}
}

func TestServiceAddAndList(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"untriaged", "parked", "dropped", "research"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o755)
	}

	svc := ideas.NewService(dir)

	idea := &ideas.Idea{
		Slug:  "my-idea",
		Title: "My Idea",
		Type:  "wild-idea",
		Body:  "# My Idea\n\nThis is wild.",
	}
	if err := svc.Add(idea); err != nil {
		t.Fatalf("add: %v", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(list))
	}
	if list[0].Status != "untriaged" {
		t.Errorf("status: got %q, want 'untriaged'", list[0].Status)
	}
}

func TestTriagePark(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"untriaged", "parked", "dropped", "research"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o755)
	}

	svc := ideas.NewService(dir)
	svc.Add(&ideas.Idea{
		Slug:  "park-me",
		Title: "Park Me",
		Body:  "# Park Me",
	})

	if err := svc.Triage("park-me", "park", "", ""); err != nil {
		t.Fatalf("triage park: %v", err)
	}

	// Should now be in parked.
	idea, err := svc.Get("park-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if idea.Status != "parked" {
		t.Errorf("status: got %q, want 'parked'", idea.Status)
	}
}

func TestTriageAssign(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"untriaged", "parked", "dropped", "research"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o755)
	}

	projectsDir := t.TempDir()
	projDir := filepath.Join(projectsDir, "myproj")
	os.MkdirAll(projDir, 0o755)

	svc := ideas.NewService(dir)
	svc.Add(&ideas.Idea{
		Slug:  "assign-me",
		Title: "Assign Me",
		Date:  "2026-03-14",
		Body:  "# Assign Me\n\nSome detail.",
	})

	if err := svc.Triage("assign-me", "assign", "myproj", projectsDir); err != nil {
		t.Fatalf("triage assign: %v", err)
	}

	// Idea file should be gone.
	if _, err := svc.Get("assign-me"); err == nil {
		t.Error("expected idea to be removed after assign")
	}

	// Backlog should exist in project.
	data, err := os.ReadFile(filepath.Join(projDir, "backlog.md"))
	if err != nil {
		t.Fatalf("reading backlog: %v", err)
	}
	if !contains(string(data), "Assign Me") {
		t.Error("backlog should contain the assigned idea title")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCheck(s, substr))
}

func containsCheck(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
