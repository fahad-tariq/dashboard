package tracker

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ItemType distinguishes tasks from goals.
type ItemType string

const (
	TaskType ItemType = "task"
	GoalType ItemType = "goal"
)

// Item represents a single tracker entry -- either a task or a goal.
type Item struct {
	Slug      string
	Title     string
	Type      ItemType
	Priority  string   // "high", "medium", "low", or ""
	Current   float64  // goals only
	Target    float64  // goals only
	Unit      string   // goals only (e.g. "kg", "books")
	Done      bool
	Graduated bool
	Body      string
	Added     string   // date added, YYYY-MM-DD
	Completed string   // date completed, YYYY-MM-DD
	Tags      []string // tags for categorisation and filtering
}

// HasTag returns true if the item has the given tag.
func (it *Item) HasTag(tag string) bool {
	for _, t := range it.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Summary holds aggregate counts for the dashboard stats row.
type Summary struct {
	OpenTasks   int
	ActiveGoals int
}

var goalRe = regexp.MustCompile(`\[goal:\s*([\d.]+)\s*/\s*([\d.]+)\s*(.*?)\]`)
var addedRe = regexp.MustCompile(`\[added:\s*(\d{4}-\d{2}-\d{2})\]`)
var completedRe = regexp.MustCompile(`\[completed:\s*(\d{4}-\d{2}-\d{2})\]`)
var tagsRe = regexp.MustCompile(`\[tags:\s*(.*?)\]`)

// ParseTracker reads a tracker.md file and returns structured items.
func ParseTracker(path string) ([]Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []Item
	var current *Item
	category := ""

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		// Section headers: ## Category
		if sectionName, ok := strings.CutPrefix(trimmed, "## "); ok {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				items = append(items, *current)
				current = nil
			}
			category = sectionName
			continue
		}

		// Skip top-level heading.
		if strings.HasPrefix(trimmed, "# ") {
			continue
		}

		// Checkbox lines: - [ ] Title or - [x] Title.
		if title, done := parseCheckbox(trimmed); title != "" {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				items = append(items, *current)
			}
			current = parseItemLine(title, done, category)
			continue
		}

		if current == nil {
			continue
		}

		// Body text (indented continuation lines).
		if trimmed != "" {
			if current.Body != "" {
				current.Body += "\n"
			}
			current.Body += trimmed
		}
	}

	if current != nil {
		current.Body = strings.TrimSpace(current.Body)
		items = append(items, *current)
	}

	return items, nil
}

// parseCheckbox extracts the title and done status from a checkbox line.
func parseCheckbox(line string) (title string, done bool) {
	rest, ok := strings.CutPrefix(line, "- ")
	if !ok {
		return "", false
	}
	switch {
	case strings.HasPrefix(rest, "[ ] "):
		return rest[4:], false
	case strings.HasPrefix(rest, "[x] "), strings.HasPrefix(rest, "[X] "):
		return rest[4:], true
	default:
		return "", false
	}
}

// parseItemLine builds an Item from the title text after the checkbox.
func parseItemLine(raw string, done bool, sectionTag string) *Item {
	item := &Item{
		Type: TaskType,
		Done: done,
	}

	title := raw

	// Extract goal metadata: [goal: current/target unit]
	if m := goalRe.FindStringSubmatch(title); m != nil {
		item.Type = GoalType
		item.Current, _ = strconv.ParseFloat(m[1], 64)
		item.Target, _ = strconv.ParseFloat(m[2], 64)
		item.Unit = strings.TrimSpace(m[3])
		title = strings.TrimSpace(goalRe.ReplaceAllString(title, ""))
	}

	// Extract priority: !high, !medium, !low
	for _, p := range []string{"high", "medium", "low"} {
		tag := "!" + p
		if strings.Contains(title, tag) {
			item.Priority = p
			title = strings.TrimSpace(strings.Replace(title, tag, "", 1))
			break
		}
	}

	// Extract added date: [added: YYYY-MM-DD]
	if m := addedRe.FindStringSubmatch(title); m != nil {
		item.Added = m[1]
		title = strings.TrimSpace(addedRe.ReplaceAllString(title, ""))
	}

	// Extract completed date: [completed: YYYY-MM-DD]
	if m := completedRe.FindStringSubmatch(title); m != nil {
		item.Completed = m[1]
		title = strings.TrimSpace(completedRe.ReplaceAllString(title, ""))
	}

	// Extract tags: [tags: tech, study]
	if m := tagsRe.FindStringSubmatch(title); m != nil {
		for _, t := range strings.Split(m[1], ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				item.Tags = append(item.Tags, t)
			}
		}
		title = strings.TrimSpace(tagsRe.ReplaceAllString(title, ""))
	}

	// Extract graduated marker.
	if strings.Contains(title, "[graduated]") {
		item.Graduated = true
		title = strings.TrimSpace(strings.Replace(title, "[graduated]", "", 1))
	}

	item.Title = strings.TrimSpace(title)
	item.Slug = Slugify(item.Title)

	// Merge section header as a tag if not already present from [tags:].
	if sectionTag != "" && !item.HasTag(sectionTag) {
		item.Tags = append([]string{sectionTag}, item.Tags...)
	}

	return item
}

// WriteTracker writes items back to a tracker.md file as a flat list.
// Tags are stored inline on each item via [tags:].
func WriteTracker(path string, items []Item) error {
	var sb strings.Builder
	sb.WriteString("# Tracker\n\n")

	for _, it := range items {
		writeItem(&sb, it)
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func writeItem(sb *strings.Builder, it Item) {
	check := "[ ]"
	if it.Done {
		check = "[x]"
	}

	fmt.Fprintf(sb, "- %s %s", check, it.Title)

	if it.Priority != "" {
		sb.WriteString(" !" + it.Priority)
	}
	if it.Type == GoalType {
		fmt.Fprintf(sb, " [goal: %s/%s", formatNum(it.Current), formatNum(it.Target))
		if it.Unit != "" {
			sb.WriteString(" " + it.Unit)
		}
		sb.WriteString("]")
	}
	if it.Added != "" {
		sb.WriteString(" [added: " + it.Added + "]")
	}
	if it.Completed != "" {
		sb.WriteString(" [completed: " + it.Completed + "]")
	}
	if len(it.Tags) > 0 {
		sb.WriteString(" [tags: " + strings.Join(it.Tags, ", ") + "]")
	}
	if it.Graduated {
		sb.WriteString(" [graduated]")
	}
	sb.WriteString("\n")

	if it.Body != "" {
		for bodyLine := range strings.SplitSeq(it.Body, "\n") {
			sb.WriteString("  " + bodyLine + "\n")
		}
	}
}

// formatNum outputs a float without trailing zeros.
func formatNum(f float64) string {
	if f == float64(int(f)) {
		return strconv.Itoa(int(f))
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ParseQuickAdd parses a quick-add input string into a task Item.
// Syntax: title #tag !priority
func ParseQuickAdd(input string) Item {
	input = strings.TrimSpace(input)
	item := Item{
		Type: TaskType,
	}

	parts := strings.Fields(input)
	var titleParts []string
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "#") && len(p) > 1:
			item.Tags = append(item.Tags, p[1:])
		case p == "!high" || p == "!medium" || p == "!low":
			item.Priority = p[1:]
		default:
			titleParts = append(titleParts, p)
		}
	}

	item.Title = strings.Join(titleParts, " ")
	item.Slug = Slugify(item.Title)
	return item
}

// Slugify converts a title to a URL-safe slug.
// Strips path traversal characters and all non-alphanumeric/hyphen chars.
func Slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
