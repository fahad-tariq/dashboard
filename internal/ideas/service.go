package ideas

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Service manages ideas stored in a single flat-file (ideas.md).
// Follows the tracker's read-modify-write pattern: parse, mutate, write back.
type Service struct {
	ideasPath string
	mu        sync.RWMutex
}

// NewService creates a new ideas service operating on the given ideas.md file path.
func NewService(ideasPath string) *Service {
	return &Service{ideasPath: ideasPath}
}

// List returns all ideas parsed from the ideas.md file.
func (s *Service) List() ([]Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ParseIdeas(s.ideasPath)
}

// Get returns a single idea by slug.
func (s *Service) Get(slug string) (*Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return nil, err
	}

	for i := range ideas {
		if ideas[i].Slug == slug {
			return &ideas[i], nil
		}
	}
	return nil, fmt.Errorf("idea %q not found", slug)
}

// Add appends a new idea to the ideas.md file.
func (s *Service) Add(idea *Idea) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idea.Added == "" {
		idea.Added = time.Now().Format("2006-01-02")
	}
	if idea.Status == "" {
		idea.Status = "untriaged"
	}

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	ideas = append(ideas, *idea)
	return WriteIdeas(s.ideasPath, "Ideas", ideas)
}

// Triage updates the status of an idea (park, drop, untriage).
func (s *Service) Triage(slug, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	for i := range ideas {
		if ideas[i].Slug == slug {
			switch action {
			case "park":
				ideas[i].Status = "parked"
			case "drop":
				ideas[i].Status = "dropped"
			case "untriage":
				ideas[i].Status = "untriaged"
			default:
				return fmt.Errorf("unknown triage action %q", action)
			}
			return WriteIdeas(s.ideasPath, "Ideas", ideas)
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

// Edit updates an idea's body, tags, and images.
func (s *Service) Edit(slug, body string, tags, images []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	for i := range ideas {
		if ideas[i].Slug == slug {
			ideas[i].Body = body
			ideas[i].Tags = tags
			ideas[i].Images = images

			// Extract title from first # heading in body if present.
			for line := range strings.SplitSeq(body, "\n") {
				if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
					ideas[i].Title = title
					ideas[i].Slug = Slugify(title)
					break
				}
			}

			return WriteIdeas(s.ideasPath, "Ideas", ideas)
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

// Delete removes an idea by slug.
func (s *Service) Delete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	for i := range ideas {
		if ideas[i].Slug == slug {
			ideas = append(ideas[:i], ideas[i+1:]...)
			return WriteIdeas(s.ideasPath, "Ideas", ideas)
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

// AddResearch appends research content to an idea's body under a ## Research heading.
func (s *Service) AddResearch(slug string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	for i := range ideas {
		if ideas[i].Slug == slug {
			if !strings.Contains(ideas[i].Body, "## Research") {
				if ideas[i].Body != "" {
					ideas[i].Body += "\n\n"
				}
				ideas[i].Body += "## Research\n\n"
			} else {
				ideas[i].Body += "\n\n"
			}
			ideas[i].Body += content
			return WriteIdeas(s.ideasPath, "Ideas", ideas)
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

// GetResearch returns the idea's body (research is inline).
// This exists for API compatibility.
func (s *Service) GetResearch(slug string) ([]byte, error) {
	idea, err := s.Get(slug)
	if err != nil {
		return nil, err
	}
	return []byte(idea.Body), nil
}
