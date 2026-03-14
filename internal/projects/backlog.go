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

		// Section headers (## Active, ## Done).
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

		if current == nil {
			continue
		}

		// Metadata lines (- key: value).
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
