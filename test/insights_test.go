package test

import (
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/insights"
)

func TestAgeBadge(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		added     string
		wantLabel string
		wantLevel string
	}{
		{"today", "2026-03-17", "0d", "fresh"},
		{"3 days ago", "2026-03-14", "3d", "fresh"},
		{"6 days ago", "2026-03-11", "6d", "fresh"},
		{"7 days ago", "2026-03-10", "7d", "ageing"},
		{"13 days ago", "2026-03-04", "13d", "ageing"},
		{"14 days ago", "2026-03-03", "14d", "stale"},
		{"29 days ago", "2026-02-16", "29d", "stale"},
		{"30 days ago", "2026-02-15", "30d", "old"},
		{"90 days ago", "2025-12-17", "3mo", "old"},
		{"empty added", "", "", ""},
		{"invalid date", "not-a-date", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, level := insights.AgeBadge(tt.added, now)
			if label != tt.wantLabel {
				t.Errorf("label: got %q, want %q", label, tt.wantLabel)
			}
			if level != tt.wantLevel {
				t.Errorf("level: got %q, want %q", level, tt.wantLevel)
			}
		})
	}
}

func TestWeeklyVelocity(t *testing.T) {
	// Wednesday 2026-03-18 -- week starts Monday 2026-03-16.
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	items := []insights.CompletedItem{
		{Completed: "2026-03-18", Done: true}, // this week
		{Completed: "2026-03-16", Done: true}, // this week (Monday)
		{Completed: "2026-03-15", Done: true}, // last week (Sunday)
		{Completed: "2026-03-10", Done: true}, // last week (Tuesday)
		{Completed: "2026-03-08", Done: true}, // two weeks ago
		{Completed: "2026-03-17", Done: false}, // not done
		{Completed: "", Done: true},             // no date
	}

	v := insights.WeeklyVelocity(items, now)
	if v.ThisWeek != 2 {
		t.Errorf("this week: got %d, want 2", v.ThisWeek)
	}
	if v.LastWeek != 2 {
		t.Errorf("last week: got %d, want 2", v.LastWeek)
	}
}

func TestWeeklyVelocity_Empty(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	v := insights.WeeklyVelocity(nil, now)
	if v.ThisWeek != 0 || v.LastWeek != 0 {
		t.Errorf("expected 0/0, got %d/%d", v.ThisWeek, v.LastWeek)
	}
	if v.String() != "No completions recently." {
		t.Errorf("string: got %q", v.String())
	}
}

func TestVelocityInsight_String(t *testing.T) {
	tests := []struct {
		name string
		v    insights.VelocityInsight
		want string
	}{
		{"zero", insights.VelocityInsight{0, 0}, "No completions recently."},
		{"only this week", insights.VelocityInsight{3, 0}, "3 completed this week."},
		{"up", insights.VelocityInsight{5, 3}, "5 completed this week, up from 3 last week."},
		{"down", insights.VelocityInsight{2, 5}, "2 completed this week, down from 5 last week."},
		{"same", insights.VelocityInsight{3, 3}, "3 completed this week, same as last week."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStreak(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		items       []insights.CompletedItem
		wantCurrent int
		wantTotal   int
	}{
		{
			"three day streak ending today",
			[]insights.CompletedItem{
				{Completed: "2026-03-17", Done: true},
				{Completed: "2026-03-16", Done: true},
				{Completed: "2026-03-15", Done: true},
				{Completed: "2026-03-13", Done: true}, // gap
			},
			3, 4,
		},
		{
			"streak ending yesterday",
			[]insights.CompletedItem{
				{Completed: "2026-03-16", Done: true},
				{Completed: "2026-03-15", Done: true},
			},
			2, 2,
		},
		{
			"no streak -- last completion 2 days ago",
			[]insights.CompletedItem{
				{Completed: "2026-03-15", Done: true},
			},
			0, 1,
		},
		{
			"empty",
			nil,
			0, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, total := insights.Streak(tt.items, now)
			if current != tt.wantCurrent {
				t.Errorf("current: got %d, want %d", current, tt.wantCurrent)
			}
			if total != tt.wantTotal {
				t.Errorf("total: got %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestMilestoneBadge(t *testing.T) {
	tests := []struct {
		total int
		want  string
	}{
		{5, ""},
		{10, "10 completed"},
		{49, "10 completed"},
		{50, "50 completed"},
		{99, "50 completed"},
		{100, "100 completed"},
		{500, "500 completed"},
	}
	for _, tt := range tests {
		got := insights.MilestoneBadge(tt.total)
		if got != tt.want {
			t.Errorf("MilestoneBadge(%d) = %q, want %q", tt.total, got, tt.want)
		}
	}
}

func TestGoalPace(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		current  float64
		target   float64
		added    string
		deadline string
		contains string
	}{
		{"no deadline", 5, 10, "2026-03-01", "", ""},
		{"target reached", 10, 10, "2026-03-01", "2026-04-01", "Target reached"},
		{"overdue", 5, 10, "2026-02-01", "2026-03-16", "Overdue"},
		{"on pace", 5, 10, "2026-03-07", "2026-03-27", "pace"},
		{"ahead", 8, 10, "2026-03-01", "2026-04-01", "Ahead"},
		{"behind", 1, 10, "2026-01-01", "2026-04-01", "Behind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insights.GoalPace(tt.current, tt.target, tt.added, tt.deadline, now)
			if tt.contains == "" && got != "" {
				t.Errorf("expected empty, got %q", got)
			}
			if tt.contains != "" && got == "" {
				t.Errorf("expected string containing %q, got empty", tt.contains)
			}
			if tt.contains != "" && len(got) > 0 {
				found := false
				for _, word := range []string{tt.contains} {
					if contains(got, word) {
						found = true
					}
				}
				if !found {
					t.Errorf("expected %q to contain %q", got, tt.contains)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestProgressColour(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		current  float64
		target   float64
		added    string
		deadline string
		want     string
	}{
		{"no deadline", 5, 10, "2026-03-01", "", "progress-fill-green"},
		{"ahead of schedule", 8, 10, "2026-03-01", "2026-04-01", "progress-fill-green"},
		{"behind schedule", 1, 10, "2026-01-01", "2026-04-01", "progress-fill-red"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insights.ProgressColour(tt.current, tt.target, tt.added, tt.deadline, now)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTagAggregation(t *testing.T) {
	items := []insights.TagInfo{
		{Tags: []string{"health", "fitness"}, Type: "task", Done: false},
		{Tags: []string{"health"}, Type: "task", Done: true},
		{Tags: []string{"health"}, Type: "goal", Done: false},
		{Tags: []string{"tech"}, Type: "idea", Done: false},
		{Tags: []string{"health", "tech"}, Type: "idea", Done: false},
	}

	result := insights.TagAggregation(items)
	if len(result) < 2 {
		t.Fatalf("expected at least 2 tags, got %d", len(result))
	}

	byTag := map[string]insights.TagSummary{}
	for _, s := range result {
		byTag[s.Tag] = s
	}

	health := byTag["health"]
	if health.TaskCount != 2 {
		t.Errorf("health tasks: got %d, want 2", health.TaskCount)
	}
	if health.GoalCount != 1 {
		t.Errorf("health goals: got %d, want 1", health.GoalCount)
	}
	if health.IdeaCount != 1 {
		t.Errorf("health ideas: got %d, want 1", health.IdeaCount)
	}
	// 1 done task out of 2 tasks + 1 goal = 3 total => 33%
	if health.CompletedPct != 33 {
		t.Errorf("health completed pct: got %d, want 33", health.CompletedPct)
	}

	tech := byTag["tech"]
	if tech.IdeaCount != 2 {
		t.Errorf("tech ideas: got %d, want 2", tech.IdeaCount)
	}
}

func TestTopN(t *testing.T) {
	summaries := []insights.TagSummary{
		{Tag: "a", TaskCount: 5},
		{Tag: "b", TaskCount: 3},
		{Tag: "c", TaskCount: 1},
	}

	top2 := insights.TopN(summaries, 2)
	if len(top2) != 2 {
		t.Fatalf("expected 2, got %d", len(top2))
	}

	topAll := insights.TopN(summaries, 10)
	if len(topAll) != 3 {
		t.Fatalf("expected 3, got %d", len(topAll))
	}
}
