package house

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fahad/dashboard/internal/slug"
)

// MaintenanceItem represents a recurring maintenance task.
// Maintenance items are never "done" -- completing them adds a log entry.
type MaintenanceItem struct {
	Slug      string
	Title     string
	Cadence   string   // raw cadence string: "3m", "2w", "90d", "1y"
	Tags      []string
	Images    []string
	Added     string // YYYY-MM-DD
	DeletedAt string // soft-delete date, YYYY-MM-DD
	Notes     string // free-text notes
	Log       []LogEntry
}

// LogEntry records a single maintenance completion.
type LogEntry struct {
	Date string // YYYY-MM-DD
	Note string // optional text after the date
}

var (
	cadenceRe = regexp.MustCompile(`\[cadence:\s*(\d+[dwmy])\]`)
	tagsRe    = regexp.MustCompile(`\[tags:\s*(.*?)\]`)
	addedRe   = regexp.MustCompile(`\[added:\s*(\d{4}-\d{2}-\d{2})\]`)
	deletedRe = regexp.MustCompile(`\[deleted:\s*(\d{4}-\d{2}-\d{2})\]`)
	imagesRe  = regexp.MustCompile(`\[images:\s*(.*?)\]`)

	logEntryRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:\s*-\s*(.*))?$`)
)

// ParseMaintenance reads a maintenance.md file and returns all items.
// Log entries are stored in file order -- the writer outputs newest first,
// and NextDue relies on Log[0] being the most recent completion.
func ParseMaintenance(path string) ([]MaintenanceItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []MaintenanceItem
	var current *MaintenanceItem

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		// Non-indented headings end the current item and are skipped.
		if !strings.HasPrefix(line, " ") && strings.HasPrefix(trimmed, "#") {
			if current != nil {
				items = append(items, *current)
				current = nil
			}
			continue
		}

		// Checkbox line starts a new item -- but only non-indented lines.
		// Indented checkbox lines (log entries) are body content.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if title, ok := parseCheckbox(trimmed); ok {
				if current != nil {
					items = append(items, *current)
				}
				current = parseMaintLine(title)
				continue
			}
		}

		if current == nil {
			continue
		}

		// Body lines: indented (2+ spaces). Parse as log entries or notes.
		if strings.HasPrefix(line, "  ") {
			bodyContent := strings.TrimSpace(line[2:])
			if entry, ok := parseLogEntry(bodyContent); ok {
				current.Log = append(current.Log, entry)
			} else if bodyContent != "" {
				if current.Notes != "" {
					current.Notes += "\n"
				}
				current.Notes += line[2:]
			}
		}
	}

	if current != nil {
		items = append(items, *current)
	}

	return items, nil
}

// parseCheckbox returns the content after a checkbox prefix and true if the line
// is a checkbox. The bool means "is a checkbox line", NOT "is done" -- maintenance
// items are never done, so the checked/unchecked state is irrelevant here.
func parseCheckbox(line string) (string, bool) {
	for _, prefix := range []string{"- [ ] ", "- [x] ", "- [X] "} {
		if strings.HasPrefix(line, prefix) {
			return line[len(prefix):], true
		}
	}
	return "", false
}

func parseMaintLine(raw string) *MaintenanceItem {
	item := &MaintenanceItem{}

	if m := cadenceRe.FindStringSubmatch(raw); m != nil {
		item.Cadence = m[1]
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := tagsRe.FindStringSubmatch(raw); m != nil {
		for t := range strings.SplitSeq(m[1], ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				item.Tags = append(item.Tags, t)
			}
		}
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := addedRe.FindStringSubmatch(raw); m != nil {
		item.Added = m[1]
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := imagesRe.FindStringSubmatch(raw); m != nil {
		for img := range strings.SplitSeq(m[1], ",") {
			img = strings.TrimSpace(img)
			if img != "" {
				item.Images = append(item.Images, img)
			}
		}
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := deletedRe.FindStringSubmatch(raw); m != nil {
		item.DeletedAt = m[1]
		raw = strings.Replace(raw, m[0], "", 1)
	}

	item.Title = strings.TrimSpace(raw)
	item.Slug = Slugify(item.Title)

	return item
}

// parseLogEntry tries to parse a body line as a completion log entry.
// Expected formats: "- [x] 2026-03-15" or "- [x] 2026-03-15 - some note"
func parseLogEntry(line string) (LogEntry, bool) {
	// Must start with a checked checkbox prefix.
	rest := ""
	for _, prefix := range []string{"- [x] ", "- [X] "} {
		if strings.HasPrefix(line, prefix) {
			rest = line[len(prefix):]
			break
		}
	}
	if rest == "" {
		return LogEntry{}, false
	}

	m := logEntryRe.FindStringSubmatch(rest)
	if m == nil {
		return LogEntry{}, false
	}

	return LogEntry{Date: m[1], Note: strings.TrimSpace(m[2])}, true
}

// WriteMaintenance writes maintenance items to a markdown file.
func WriteMaintenance(path, heading string, items []MaintenanceItem) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", heading)

	for i, it := range items {
		b.WriteString("- [ ] ")
		b.WriteString(it.Title)

		if it.Cadence != "" {
			fmt.Fprintf(&b, " [cadence: %s]", it.Cadence)
		}
		if len(it.Tags) > 0 {
			fmt.Fprintf(&b, " [tags: %s]", strings.Join(it.Tags, ", "))
		}
		if it.Added != "" {
			fmt.Fprintf(&b, " [added: %s]", it.Added)
		}
		if len(it.Images) > 0 {
			fmt.Fprintf(&b, " [images: %s]", strings.Join(it.Images, ", "))
		}
		if it.DeletedAt != "" {
			fmt.Fprintf(&b, " [deleted: %s]", it.DeletedAt)
		}
		b.WriteString("\n")

		if it.Notes != "" {
			for noteLine := range strings.SplitSeq(it.Notes, "\n") {
				b.WriteString("  ")
				b.WriteString(noteLine)
				b.WriteString("\n")
			}
		}

		for _, entry := range it.Log {
			b.WriteString("  - [x] ")
			b.WriteString(entry.Date)
			if entry.Note != "" {
				b.WriteString(" - ")
				b.WriteString(entry.Note)
			}
			b.WriteString("\n")
		}

		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Slugify exposes the shared slug generation.
func Slugify(title string) string {
	return slug.Slugify(title)
}

// ParseCadence parses a cadence string like "3m", "2w", "90d", "1y".
// Returns the numeric amount and unit byte ('d', 'w', 'm', 'y').
func ParseCadence(s string) (int, byte, error) {
	if len(s) < 2 {
		return 0, 0, fmt.Errorf("cadence too short: %q", s)
	}
	unit := s[len(s)-1]
	switch unit {
	case 'd', 'w', 'm', 'y':
	default:
		return 0, 0, fmt.Errorf("invalid cadence unit %q in %q", string(unit), s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cadence number in %q: %w", s, err)
	}
	if n < 1 {
		return 0, 0, fmt.Errorf("cadence must be at least 1, got %d", n)
	}
	// Cap at reasonable maximums.
	maxByUnit := map[byte]int{'d': 3650, 'w': 520, 'm': 120, 'y': 10}
	if n > maxByUnit[unit] {
		return 0, 0, fmt.Errorf("cadence %d%c exceeds maximum %d%c", n, unit, maxByUnit[unit], unit)
	}
	return n, unit, nil
}

// NextDue computes when the item is next due based on the most recent log entry.
// Returns the zero time if there are no log entries (always overdue).
func (it *MaintenanceItem) NextDue(loc *time.Location) time.Time {
	if len(it.Log) == 0 || it.Cadence == "" {
		return time.Time{}
	}

	lastDate, err := time.ParseInLocation("2006-01-02", it.Log[0].Date, loc)
	if err != nil {
		return time.Time{}
	}

	n, unit, err := ParseCadence(it.Cadence)
	if err != nil {
		return time.Time{}
	}

	switch unit {
	case 'd':
		return lastDate.AddDate(0, 0, n)
	case 'w':
		return lastDate.AddDate(0, 0, n*7)
	case 'm':
		return lastDate.AddDate(0, n, 0)
	case 'y':
		return lastDate.AddDate(n, 0, 0)
	}
	return time.Time{}
}

// IsOverdue returns true if the item's next-due date is on or before now.
// Items with no log entries are always overdue.
func (it *MaintenanceItem) IsOverdue(now time.Time, loc *time.Location) bool {
	due := it.NextDue(loc)
	if due.IsZero() {
		return true // never done
	}
	return !due.After(now)
}

// DaysUntilDue returns negative if overdue, 0 if due today, positive if upcoming.
func (it *MaintenanceItem) DaysUntilDue(now time.Time, loc *time.Location) int {
	due := it.NextDue(loc)
	if due.IsZero() {
		return -9999 // never done
	}
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dueDate := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, loc)
	return int(dueDate.Sub(nowDate).Hours() / 24)
}
