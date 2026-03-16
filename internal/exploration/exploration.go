package exploration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fahad/dashboard/internal/slug"
)

type Exploration struct {
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags,omitempty"`
	Date   string   `json:"date,omitempty"`
	Images []string `json:"images,omitempty"`
	Body   string   `json:"body"`
}

// ParseExploration reads a single exploration markdown file.
func ParseExploration(path string) (*Exploration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	e := &Exploration{
		Slug: slugFromPath(path),
	}

	content := string(data)
	frontmatter, body := splitFrontmatter(content)

	for line := range strings.SplitSeq(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ": "); ok {
			switch k {
			case "tags":
				for t := range strings.SplitSeq(v, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						e.Tags = append(e.Tags, t)
					}
				}
			case "date":
				e.Date = v
			case "images":
				for img := range strings.SplitSeq(v, ",") {
					img = strings.TrimSpace(img)
					if img != "" {
						e.Images = append(e.Images, img)
					}
				}
			}
		}
	}

	for line := range strings.SplitSeq(body, "\n") {
		if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			e.Title = title
			break
		}
	}

	e.Body = strings.TrimSpace(body)
	return e, nil
}

// WriteExploration writes an exploration to a markdown file.
func WriteExploration(dir string, e *Exploration) error {
	var b strings.Builder
	b.WriteString("---\n")
	if len(e.Tags) > 0 {
		fmt.Fprintf(&b, "tags: %s\n", strings.Join(e.Tags, ", "))
	}
	if e.Date != "" {
		fmt.Fprintf(&b, "date: %s\n", e.Date)
	}
	if len(e.Images) > 0 {
		fmt.Fprintf(&b, "images: %s\n", strings.Join(e.Images, ", "))
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	b.WriteString("\n")

	path := filepath.Join(dir, e.Slug+".md")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ParseQuickAdd extracts #tag tokens from input, returning title and tags.
func ParseQuickAdd(input string) (title string, tags []string) {
	parts := strings.Fields(input)
	var titleParts []string
	for _, p := range parts {
		if strings.HasPrefix(p, "#") && len(p) > 1 {
			tags = append(tags, p[1:])
		} else {
			titleParts = append(titleParts, p)
		}
	}
	title = strings.Join(titleParts, " ")
	return
}

func splitFrontmatter(content string) (frontmatter, body string) {
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

// Slugify exposes the shared slugify for use by the handler.
func Slugify(title string) string {
	return slug.Slugify(title)
}
