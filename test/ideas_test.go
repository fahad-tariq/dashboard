package test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	if !slices.Equal(idea.Tags, []string{"technical-exploration"}) {
		t.Errorf("tags: got %v, want [technical-exploration]", idea.Tags)
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
		Tags:             []string{"feature"},
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

	if !slices.Equal(parsed.Tags, original.Tags) {
		t.Errorf("tags: got %v, want %v", parsed.Tags, original.Tags)
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
		Tags:  []string{"wild-idea"},
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

	if err := svc.Triage("park-me", "park"); err != nil {
		t.Fatalf("triage park: %v", err)
	}

	idea, err := svc.Get("park-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if idea.Status != "parked" {
		t.Errorf("status: got %q, want 'parked'", idea.Status)
	}
}

func TestParseIdea_TagsMigration(t *testing.T) {
	dir := t.TempDir()
	content := `---
type: feature
date: 2026-03-14
---

# Legacy idea
`
	path := filepath.Join(dir, "legacy.md")
	os.WriteFile(path, []byte(content), 0o644)

	idea, err := ideas.ParseIdea(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !slices.Equal(idea.Tags, []string{"feature"}) {
		t.Errorf("tags: got %v, want [feature]", idea.Tags)
	}
}

func TestParseIdea_Tags(t *testing.T) {
	dir := t.TempDir()
	content := `---
tags: foo, bar
date: 2026-03-14
---

# Multi-tag idea
`
	path := filepath.Join(dir, "multi-tag.md")
	os.WriteFile(path, []byte(content), 0o644)

	idea, err := ideas.ParseIdea(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !slices.Equal(idea.Tags, []string{"foo", "bar"}) {
		t.Errorf("tags: got %v, want [foo bar]", idea.Tags)
	}
}

func TestWriteIdea_Tags(t *testing.T) {
	dir := t.TempDir()
	idea := &ideas.Idea{
		Slug:             "tagged",
		Title:            "Tagged Idea",
		Tags:             []string{"feature", "exploration"},
		SuggestedProject: "dashboard",
		Date:             "2026-03-14",
		Research:         "research/tagged.md",
		Body:             "# Tagged Idea\n\nBody here.",
	}

	if err := ideas.WriteIdea(dir, idea); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tagged.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "tags: feature, exploration") {
		t.Errorf("expected tags line, got:\n%s", content)
	}
	if strings.Contains(content, "type:") {
		t.Errorf("should not contain type: line, got:\n%s", content)
	}
	if !strings.Contains(content, "suggested-project: dashboard") {
		t.Errorf("should preserve suggested-project, got:\n%s", content)
	}
	if !strings.Contains(content, "research: research/tagged.md") {
		t.Errorf("should preserve research, got:\n%s", content)
	}

	parsed, err := ideas.ParseIdea(filepath.Join(dir, "tagged.md"))
	if err != nil {
		t.Fatalf("roundtrip parse: %v", err)
	}
	if !slices.Equal(parsed.Tags, idea.Tags) {
		t.Errorf("roundtrip tags: got %v, want %v", parsed.Tags, idea.Tags)
	}
	if parsed.SuggestedProject != idea.SuggestedProject {
		t.Errorf("roundtrip suggested-project: got %q", parsed.SuggestedProject)
	}
	if parsed.Research != idea.Research {
		t.Errorf("roundtrip research: got %q", parsed.Research)
	}
}

