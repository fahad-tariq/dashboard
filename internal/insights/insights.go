package insights

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// TagInfo is a lightweight struct for tag aggregation, avoiding direct
// dependency on tracker.Item or ideas.Idea.
type TagInfo struct {
	Tags []string
	Type string // "task", "goal", or "idea"
	Done bool
}

// TagSummary holds cross-section counts for a single tag.
type TagSummary struct {
	Tag          string
	TaskCount    int
	GoalCount    int
	IdeaCount    int
	CompletedPct int
}

// VelocityInsight holds weekly completion counts for display.
type VelocityInsight struct {
	ThisWeek int
	LastWeek int
}

// String returns a human-readable velocity summary.
func (v VelocityInsight) String() string {
	if v.ThisWeek == 0 && v.LastWeek == 0 {
		return "No completions recently."
	}
	if v.LastWeek == 0 {
		return fmt.Sprintf("%d completed this week.", v.ThisWeek)
	}
	if v.ThisWeek > v.LastWeek {
		return fmt.Sprintf("%d completed this week, up from %d last week.", v.ThisWeek, v.LastWeek)
	}
	if v.ThisWeek < v.LastWeek {
		return fmt.Sprintf("%d completed this week, down from %d last week.", v.ThisWeek, v.LastWeek)
	}
	return fmt.Sprintf("%d completed this week, same as last week.", v.ThisWeek)
}

// CompletedItem is the minimal interface needed by velocity/streak functions.
type CompletedItem struct {
	Completed string // YYYY-MM-DD
	Done      bool
}

// AgeBadge returns a human-readable age label and staleness level for an item.
// Levels: "fresh" (<7d), "ageing" (7-14d), "stale" (14-30d), "old" (30+d).
func AgeBadge(added string, now time.Time) (label string, level string) {
	t, err := time.Parse("2006-01-02", added)
	if err != nil || added == "" {
		return "", ""
	}
	days := int(now.Sub(t).Hours() / 24)
	if days < 0 {
		days = 0
	}

	switch {
	case days < 7:
		return fmt.Sprintf("%dd", days), "fresh"
	case days < 14:
		return fmt.Sprintf("%dd", days), "ageing"
	case days < 30:
		return fmt.Sprintf("%dd", days), "stale"
	default:
		if days >= 365 {
			years := days / 365
			return fmt.Sprintf("%dy", years), "old"
		}
		if days >= 60 {
			months := days / 30
			return fmt.Sprintf("%dmo", months), "old"
		}
		return fmt.Sprintf("%dd", days), "old"
	}
}

// WeeklyVelocity counts completions in the current and previous calendar weeks.
func WeeklyVelocity(items []CompletedItem, now time.Time) VelocityInsight {
	// Find start of this week (Monday).
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	thisWeekStart := now.AddDate(0, 0, -int(weekday-time.Monday))
	thisWeekStart = time.Date(thisWeekStart.Year(), thisWeekStart.Month(), thisWeekStart.Day(), 0, 0, 0, 0, now.Location())
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)

	var v VelocityInsight
	for _, it := range items {
		if !it.Done || it.Completed == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", it.Completed)
		if err != nil {
			continue
		}
		if !t.Before(thisWeekStart) {
			v.ThisWeek++
		} else if !t.Before(lastWeekStart) {
			v.LastWeek++
		}
	}
	return v
}

// Streak calculates the current consecutive-day streak and total completed count.
func Streak(items []CompletedItem, now time.Time) (current int, total int) {
	dateSet := map[string]bool{}
	for _, it := range items {
		if it.Done && it.Completed != "" {
			dateSet[it.Completed] = true
			total++
		}
	}
	if len(dateSet) == 0 {
		return 0, total
	}

	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Streak must start from today or yesterday to be "current".
	var checkDate time.Time
	if dateSet[today] {
		checkDate = now
	} else if dateSet[yesterday] {
		checkDate = now.AddDate(0, 0, -1)
	} else {
		return 0, total
	}

	for {
		ds := checkDate.Format("2006-01-02")
		if !dateSet[ds] {
			break
		}
		current++
		checkDate = checkDate.AddDate(0, 0, -1)
	}
	return current, total
}

// MilestoneBadge returns a label if the total crosses a milestone threshold.
func MilestoneBadge(total int) string {
	milestones := []int{500, 100, 50, 10}
	for _, m := range milestones {
		if total >= m {
			return fmt.Sprintf("%d completed", m)
		}
	}
	return ""
}

// GoalPace returns a pace narrative for a goal with a deadline.
func GoalPace(current, target float64, added, deadline string, now time.Time) string {
	if deadline == "" || target <= 0 {
		return ""
	}
	deadlineTime, err := time.Parse("2006-01-02", deadline)
	if err != nil {
		return ""
	}
	addedTime, err := time.Parse("2006-01-02", added)
	if err != nil {
		addedTime = now
	}

	remaining := target - current
	if remaining <= 0 {
		return "Target reached"
	}

	daysLeft := int(deadlineTime.Sub(now).Hours()/24) + 1
	if daysLeft <= 0 {
		return "Overdue"
	}

	totalDays := int(deadlineTime.Sub(addedTime).Hours()/24) + 1
	if totalDays <= 0 {
		totalDays = 1
	}

	elapsed := int(now.Sub(addedTime).Hours()/24) + 1
	if elapsed < 0 {
		elapsed = 0
	}

	expectedPct := float64(elapsed) / float64(totalDays) * 100
	actualPct := current / target * 100

	if actualPct >= expectedPct+10 {
		return fmt.Sprintf("Ahead of pace -- %d days left", daysLeft)
	}
	if actualPct <= expectedPct-10 {
		return fmt.Sprintf("Behind pace -- %d days left", daysLeft)
	}
	return fmt.Sprintf("On pace -- %d days left", daysLeft)
}

// ProgressColour returns a CSS class for goal progress bar colour based on
// deadline proximity vs percentage complete.
func ProgressColour(current, target float64, added, deadline string, now time.Time) string {
	if deadline == "" || target <= 0 {
		return "progress-fill-green"
	}
	deadlineTime, err := time.Parse("2006-01-02", deadline)
	if err != nil {
		return "progress-fill-green"
	}
	addedTime, err := time.Parse("2006-01-02", added)
	if err != nil {
		addedTime = now
	}

	totalDays := deadlineTime.Sub(addedTime).Hours() / 24
	if totalDays <= 0 {
		totalDays = 1
	}
	elapsed := now.Sub(addedTime).Hours() / 24
	timePct := elapsed / totalDays * 100
	progressPct := current / target * 100

	diff := progressPct - timePct

	switch {
	case diff >= 0:
		return "progress-fill-green"
	case diff >= -15:
		return "progress-fill-yellow"
	case diff >= -30:
		return "progress-fill-orange"
	default:
		return "progress-fill-red"
	}
}

// TagAggregation computes cross-section tag counts from tasks, goals, and ideas.
func TagAggregation(items []TagInfo) []TagSummary {
	byTag := map[string]*TagSummary{}

	for _, info := range items {
		for _, tag := range info.Tags {
			tag = strings.ToLower(tag)
			s, ok := byTag[tag]
			if !ok {
				s = &TagSummary{Tag: tag}
				byTag[tag] = s
			}
			switch info.Type {
			case "task":
				s.TaskCount++
			case "goal":
				s.GoalCount++
			case "idea":
				s.IdeaCount++
			}
		}
	}

	// Calculate completed percentage per tag.
	tagDone := map[string]int{}
	tagTotal := map[string]int{}
	for _, info := range items {
		if info.Type == "idea" {
			continue
		}
		for _, tag := range info.Tags {
			tag = strings.ToLower(tag)
			tagTotal[tag]++
			if info.Done {
				tagDone[tag]++
			}
		}
	}
	for tag, s := range byTag {
		total := tagTotal[tag]
		if total > 0 {
			s.CompletedPct = tagDone[tag] * 100 / total
		}
	}

	result := make([]TagSummary, 0, len(byTag))
	for _, s := range byTag {
		result = append(result, *s)
	}
	sort.Slice(result, func(i, j int) bool {
		ti := result[i].TaskCount + result[i].GoalCount + result[i].IdeaCount
		tj := result[j].TaskCount + result[j].GoalCount + result[j].IdeaCount
		if ti != tj {
			return ti > tj
		}
		return result[i].Tag < result[j].Tag
	})
	return result
}

// TopN returns the first n items from a TagSummary slice, or all if fewer.
func TopN(summaries []TagSummary, n int) []TagSummary {
	if len(summaries) <= n {
		return slices.Clone(summaries)
	}
	return slices.Clone(summaries[:n])
}
