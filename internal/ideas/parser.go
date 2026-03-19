package ideas

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fahad/dashboard/internal/slug"
)

// Idea represents a single idea in the flat-file ideas.md format.
type Idea struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Status      string   `json:"status"` // untriaged, parked, dropped, converted
	Tags        []string `json:"tags,omitempty"`
	Images      []string `json:"images,omitempty"`
	Project     string   `json:"project,omitempty"`
	Added       string   `json:"added,omitempty"`
	ConvertedTo string   `json:"converted_to,omitempty"` // slug of the task this idea was converted to
	Body        string   `json:"body"`
	DeletedAt   string   `json:"deleted_at,omitempty"` // soft-delete date, YYYY-MM-DD
}

var (
	statusRe      = regexp.MustCompile(`\[status:\s*(.*?)\]`)
	tagsRe        = regexp.MustCompile(`\[tags:\s*(.*?)\]`)
	projectRe     = regexp.MustCompile(`\[project:\s*(.*?)\]`)
	addedRe       = regexp.MustCompile(`\[added:\s*(\d{4}-\d{2}-\d{2})\]`)
	imagesRe      = regexp.MustCompile(`\[images:\s*(.*?)\]`)
	convertedToRe = regexp.MustCompile(`\[converted-to:\s*([\w-]+)\]`)
	deletedRe     = regexp.MustCompile(`\[deleted:\s*(\d{4}-\d{2}-\d{2})\]`)
)

// ParseIdeas reads an ideas.md file and returns all ideas.
// The format is checkbox lines with inline metadata followed by indented body lines.
// Blank lines within body blocks are preserved (unlike the tracker parser).
func ParseIdeas(path string) ([]Idea, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ideas []Idea
	var current *Idea

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		// Headings end the current idea and are skipped.
		// Only non-indented lines are headings; indented # lines are body content.
		if !strings.HasPrefix(line, " ") && strings.HasPrefix(trimmed, "#") {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				ideas = append(ideas, *current)
				current = nil
			}
			continue
		}

		// Checkbox line starts a new idea.
		if title, ok := parseIdeaCheckbox(trimmed); ok {
			if current != nil {
				current.Body = strings.TrimSpace(current.Body)
				ideas = append(ideas, *current)
			}
			current = parseIdeaLine(title)
			continue
		}

		if current == nil {
			continue
		}

		// Body lines: indented (2+ spaces) or blank lines between indented lines.
		if strings.HasPrefix(line, "  ") {
			current.Body += line[2:] + "\n"
		} else if trimmed == "" {
			current.Body += "\n"
		}
	}

	if current != nil {
		current.Body = strings.TrimSpace(current.Body)
		ideas = append(ideas, *current)
	}

	return ideas, nil
}

// parseIdeaCheckbox checks if a line is a checkbox and returns the content after the checkbox prefix.
func parseIdeaCheckbox(line string) (string, bool) {
	for _, prefix := range []string{"- [ ] ", "- [x] ", "- [X] "} {
		if strings.HasPrefix(line, prefix) {
			return line[len(prefix):], true
		}
	}
	return "", false
}

// parseIdeaLine extracts metadata from a checkbox line content and returns an Idea.
func parseIdeaLine(raw string) *Idea {
	idea := &Idea{}

	if m := statusRe.FindStringSubmatch(raw); m != nil {
		idea.Status = strings.TrimSpace(m[1])
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := tagsRe.FindStringSubmatch(raw); m != nil {
		for t := range strings.SplitSeq(m[1], ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				idea.Tags = append(idea.Tags, t)
			}
		}
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := projectRe.FindStringSubmatch(raw); m != nil {
		idea.Project = strings.TrimSpace(m[1])
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := addedRe.FindStringSubmatch(raw); m != nil {
		idea.Added = m[1]
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := imagesRe.FindStringSubmatch(raw); m != nil {
		for img := range strings.SplitSeq(m[1], ",") {
			img = strings.TrimSpace(img)
			if img != "" {
				idea.Images = append(idea.Images, img)
			}
		}
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := convertedToRe.FindStringSubmatch(raw); m != nil {
		idea.ConvertedTo = strings.TrimSpace(m[1])
		raw = strings.Replace(raw, m[0], "", 1)
	}
	if m := deletedRe.FindStringSubmatch(raw); m != nil {
		idea.DeletedAt = m[1]
		raw = strings.Replace(raw, m[0], "", 1)
	}

	idea.Title = strings.TrimSpace(raw)
	idea.Slug = slug.Slugify(idea.Title)

	if idea.Status == "" {
		idea.Status = "untriaged"
	}

	return idea
}

// WriteIdeas writes all ideas to a flat-file ideas.md.
func WriteIdeas(path string, heading string, ideas []Idea) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", heading)

	for i, idea := range ideas {
		b.WriteString("- [ ] ")
		b.WriteString(idea.Title)

		if idea.Status != "" {
			fmt.Fprintf(&b, " [status: %s]", idea.Status)
		}
		if len(idea.Tags) > 0 {
			fmt.Fprintf(&b, " [tags: %s]", strings.Join(idea.Tags, ", "))
		}
		if idea.Project != "" {
			fmt.Fprintf(&b, " [project: %s]", idea.Project)
		}
		if idea.Added != "" {
			fmt.Fprintf(&b, " [added: %s]", idea.Added)
		}
		if idea.ConvertedTo != "" {
			fmt.Fprintf(&b, " [converted-to: %s]", idea.ConvertedTo)
		}
		if len(idea.Images) > 0 {
			fmt.Fprintf(&b, " [images: %s]", strings.Join(idea.Images, ", "))
		}
		if idea.DeletedAt != "" {
			fmt.Fprintf(&b, " [deleted: %s]", idea.DeletedAt)
		}
		b.WriteString("\n")

		if idea.Body != "" {
			for bodyLine := range strings.SplitSeq(idea.Body, "\n") {
				if bodyLine == "" {
					b.WriteString("\n")
				} else {
					b.WriteString("  ")
					b.WriteString(bodyLine)
					b.WriteString("\n")
				}
			}
		}

		if i < len(ideas)-1 {
			b.WriteString("\n")
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Slugify exposes the shared slug generation for use by the handler.
func Slugify(title string) string {
	return slug.Slugify(title)
}

