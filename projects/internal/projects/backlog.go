package projects

import (
	"os"
	"path/filepath"
	"strings"
)

type BacklogItem struct {
	Title    string
	Priority string
	Added    string
	Done     string
	Plan     string
	Body     string
	Section  string // "Active" or "Done"
}

// ParseBacklog reads a project's backlog.md and returns structured items.
// Supports two formats:
//   - ### Title with - key: value metadata lines
//   - Checkbox bullets: - [ ] Title or - ☐ Title (with optional **bold** title)
func ParseBacklog(projectPath string) ([]BacklogItem, error) {
	data, err := os.ReadFile(filepath.Join(projectPath, "backlog.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []BacklogItem
	var current *BacklogItem
	section := ""

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		// Section headers (## Active, ## Done, ## Outstanding, etc.).
		if sectionName, ok := strings.CutPrefix(trimmed, "## "); ok {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				items = append(items, *current)
				current = nil
			}
			section = sectionName
			continue
		}

		// Item headers (### Title).
		if title, ok := strings.CutPrefix(trimmed, "### "); ok {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				items = append(items, *current)
			}
			current = &BacklogItem{
				Title:   title,
				Section: section,
			}
			continue
		}

		// Checkbox bullets: - [ ] Title, - [x] Title, - ☐ Title, - ☑ Title.
		if title, done := parseCheckboxLine(trimmed); title != "" {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				items = append(items, *current)
			}
			current = &BacklogItem{
				Title:   title,
				Section: section,
			}
			if done {
				current.Done = "yes"
			}
			continue
		}

		if current == nil {
			continue
		}

		// Metadata lines (- key: value) under a ### item.
		if kv, ok := strings.CutPrefix(trimmed, "- "); ok {
			if k, v, ok := strings.Cut(kv, ": "); ok {
				switch k {
				case "priority":
					current.Priority = v
				case "added":
					current.Added = v
				case "done":
					current.Done = v
				case "plan":
					current.Plan = v
				}
				continue
			}
		}

		// Body text.
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

// parseCheckboxLine parses checkbox-style bullet lines.
// Returns the title and whether the item is done (checked).
// Supports: - [ ] Title, - [x] Title, - ☐ Title, - ☑ Title.
// Also strips **bold** markers from the title.
func parseCheckboxLine(line string) (title string, done bool) {
	rest, ok := strings.CutPrefix(line, "- ")
	if !ok {
		return "", false
	}

	switch {
	case strings.HasPrefix(rest, "[ ] "):
		title = rest[4:]
	case strings.HasPrefix(rest, "[x] "), strings.HasPrefix(rest, "[X] "):
		title = rest[4:]
		done = true
	case strings.HasPrefix(rest, "☐ "):
		title = rest[len("☐ "):]
	case strings.HasPrefix(rest, "☑ "):
		title = rest[len("☑ "):]
		done = true
	default:
		return "", false
	}

	// Strip leading **bold** markers from title.
	if after, ok := strings.CutPrefix(title, "**"); ok {
		if end := strings.Index(after, "**"); end >= 0 {
			title = after[:end]
		}
	}

	// Trim trailing description after " -- " or " — ".
	if idx := strings.Index(title, " -- "); idx >= 0 {
		title = title[:idx]
	}
	if idx := strings.Index(title, " — "); idx >= 0 {
		title = title[:idx]
	}

	return strings.TrimSpace(title), done
}

// ListPlans returns the filenames of plans/*.md in a project directory.
func ListPlans(projectPath string) []string {
	plansDir := filepath.Join(projectPath, "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return nil
	}

	var plans []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			plans = append(plans, e.Name())
		}
	}
	return plans
}
