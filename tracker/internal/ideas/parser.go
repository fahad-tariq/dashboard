package ideas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Idea struct {
	Slug             string `json:"slug"`
	Title            string `json:"title"`
	Type             string `json:"type,omitempty"`
	SuggestedProject string `json:"suggested_project,omitempty"`
	Date             string `json:"date,omitempty"`
	Research         string `json:"research,omitempty"`
	Body             string `json:"body"`
	Status           string `json:"status"` // untriaged, parked, dropped
}

// ParseIdea reads a single idea markdown file and returns a structured Idea.
// The file format is frontmatter (---) followed by markdown body.
func ParseIdea(path string) (*Idea, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	idea := &Idea{
		Slug: slugFromPath(path),
	}

	content := string(data)
	frontmatter, body := splitFrontmatter(content)

	for line := range strings.SplitSeq(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ": "); ok {
			switch k {
			case "type":
				idea.Type = v
			case "suggested-project":
				idea.SuggestedProject = v
			case "date":
				idea.Date = v
			case "research":
				idea.Research = v
			}
		}
	}

	// Extract title from first # heading in body.
	for line := range strings.SplitSeq(body, "\n") {
		if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			idea.Title = title
			break
		}
	}

	idea.Body = strings.TrimSpace(body)

	return idea, nil
}

// WriteIdea writes an idea to a markdown file in the given directory.
func WriteIdea(dir string, idea *Idea) error {
	var b strings.Builder
	b.WriteString("---\n")
	if idea.Type != "" {
		fmt.Fprintf(&b, "type: %s\n", idea.Type)
	}
	if idea.SuggestedProject != "" {
		fmt.Fprintf(&b, "suggested-project: %s\n", idea.SuggestedProject)
	}
	if idea.Date != "" {
		fmt.Fprintf(&b, "date: %s\n", idea.Date)
	}
	if idea.Research != "" {
		fmt.Fprintf(&b, "research: %s\n", idea.Research)
	}
	b.WriteString("---\n\n")
	b.WriteString(idea.Body)
	b.WriteString("\n")

	path := filepath.Join(dir, idea.Slug+".md")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func splitFrontmatter(content string) (frontmatter, body string) {
	// Expect "---\n...frontmatter...\n---\n...body..."
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[4:]
	fm, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", content
	}
	return fm, strings.TrimPrefix(after, "\n")
}

func slugFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".md")
}
