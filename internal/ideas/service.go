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
	cache     []Idea
}

// NewService creates a new ideas service operating on the given ideas.md file path.
func NewService(ideasPath string) *Service {
	s := &Service{ideasPath: ideasPath}
	s.loadCache()
	return s
}

func (s *Service) loadCache() {
	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		ideas = nil
	}
	s.cache = ideas
}

// List returns all ideas from the in-memory cache.
func (s *Service) List() ([]Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Idea, len(s.cache))
	copy(out, s.cache)
	return out, nil
}

// Get returns a single idea by slug.
func (s *Service) Get(slug string) (*Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.cache {
		if s.cache[i].Slug == slug {
			cp := s.cache[i]
			return &cp, nil
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
	if err := WriteIdeas(s.ideasPath, "Ideas", ideas); err != nil {
		return err
	}
	s.cache = ideas
	return nil
}

func (s *Service) mutate(slug string, fn func(*Idea) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	for i := range ideas {
		if ideas[i].Slug == slug {
			if err := fn(&ideas[i]); err != nil {
				return err
			}
			if err := WriteIdeas(s.ideasPath, "Ideas", ideas); err != nil {
				return err
			}
			s.cache = ideas
			return nil
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

func (s *Service) Triage(slug, action string) error {
	return s.mutate(slug, func(idea *Idea) error {
		switch action {
		case "park":
			idea.Status = "parked"
		case "drop":
			idea.Status = "dropped"
		case "untriage":
			idea.Status = "untriaged"
		default:
			return fmt.Errorf("unknown triage action %q", action)
		}
		return nil
	})
}

func (s *Service) Edit(slug, body string, tags, images []string) error {
	return s.mutate(slug, func(idea *Idea) error {
		idea.Body = body
		idea.Tags = tags
		idea.Images = images
		for line := range strings.SplitSeq(body, "\n") {
			if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
				idea.Title = title
				idea.Slug = Slugify(title)
				break
			}
		}
		return nil
	})
}

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
			if err := WriteIdeas(s.ideasPath, "Ideas", ideas); err != nil {
				return err
			}
			s.cache = ideas
			return nil
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

func (s *Service) AddResearch(slug string, content string) error {
	return s.mutate(slug, func(idea *Idea) error {
		if !strings.Contains(idea.Body, "## Research") {
			if idea.Body != "" {
				idea.Body += "\n\n"
			}
			idea.Body += "## Research\n\n"
		} else {
			idea.Body += "\n\n"
		}
		idea.Body += content
		return nil
	})
}

// Resync refreshes the in-memory cache from disk.
func (s *Service) Resync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}
	s.cache = ideas
	return nil
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
