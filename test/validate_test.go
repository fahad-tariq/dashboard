package test

import (
	"testing"

	"github.com/fahad/dashboard/internal/httputil"
)

func TestStripInlineMetadata(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no metadata",
			input: "Fix the authentication bug",
			want:  "Fix the authentication bug",
		},
		{
			name:  "planned date",
			input: "Fix bug [planned: 2026-03-28]",
			want:  "Fix bug",
		},
		{
			name:  "deleted date",
			input: "Old task [deleted: 2026-03-20]",
			want:  "Old task",
		},
		{
			name:  "tags",
			input: "New task [tags: backend, urgent]",
			want:  "New task",
		},
		{
			name:  "status",
			input: "Idea [status: parked]",
			want:  "Idea",
		},
		{
			name:  "multiple metadata",
			input: "Task [planned: 2026-03-28] [tags: work] [deleted: 2026-01-01]",
			want:  "Task",
		},
		{
			name:  "deadline",
			input: "Goal [deadline: 2026-06-01]",
			want:  "Goal",
		},
		{
			name:  "plan-order",
			input: "Task [plan-order: 3]",
			want:  "Task",
		},
		{
			name:  "from-idea",
			input: "Task [from-idea: my-idea-slug]",
			want:  "Task",
		},
		{
			name:  "converted-to",
			input: "Idea [converted-to: task-slug]",
			want:  "Idea",
		},
		{
			name:  "goal metadata",
			input: "Run more [goal: 5/10 km]",
			want:  "Run more",
		},
		{
			name:  "images",
			input: "Task [images: abc123.jpg]",
			want:  "Task",
		},
		{
			name:  "added",
			input: "Task [added: 2026-03-01]",
			want:  "Task",
		},
		{
			name:  "completed",
			input: "Task [completed: 2026-03-15]",
			want:  "Task",
		},
		{
			name:  "preserves regular brackets",
			input: "Fix [AUTH-123] login issue",
			want:  "Fix [AUTH-123] login issue",
		},
		{
			name:  "preserves markdown links",
			input: "See [this page](https://example.com)",
			want:  "See [this page](https://example.com)",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httputil.StripInlineMetadata(tt.input)
			if got != tt.want {
				t.Errorf("StripInlineMetadata(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateList(t *testing.T) {
	tests := []struct {
		list string
		want bool
	}{
		{"personal", true},
		{"todos", true},
		{"family", true},
		{"house", true},
		{"ideas", false},
		{"", false},
		{"invalid", false},
	}
	for _, tt := range tests {
		if got := httputil.ValidateList(tt.list); got != tt.want {
			t.Errorf("ValidateList(%q) = %v, want %v", tt.list, got, tt.want)
		}
	}
}

func TestValidateListWithIdeas(t *testing.T) {
	tests := []struct {
		list string
		want bool
	}{
		{"personal", true},
		{"todos", true},
		{"family", true},
		{"ideas", true},
		{"house", true},
		{"", false},
		{"invalid", false},
	}
	for _, tt := range tests {
		if got := httputil.ValidateListWithIdeas(tt.list); got != tt.want {
			t.Errorf("ValidateListWithIdeas(%q) = %v, want %v", tt.list, got, tt.want)
		}
	}
}

func TestNormaliseList(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"todos", "personal"},
		{"personal", "personal"},
		{"family", "family"},
		{"ideas", "ideas"},
	}
	for _, tt := range tests {
		if got := httputil.NormaliseList(tt.input); got != tt.want {
			t.Errorf("NormaliseList(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
