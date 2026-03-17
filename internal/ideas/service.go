package ideas

import (
	"fmt"
	"log/slog"
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

// List returns all non-deleted ideas from the in-memory cache.
func (s *Service) List() ([]Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Idea
	for _, idea := range s.cache {
		if idea.DeletedAt == "" {
			out = append(out, idea)
		}
	}
	return out, nil
}

// ListDeleted returns only soft-deleted ideas from the cache.
func (s *Service) ListDeleted() []Idea {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Idea
	for _, idea := range s.cache {
		if idea.DeletedAt != "" {
			out = append(out, idea)
		}
	}
	return out
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

func (s *Service) Edit(slug, title, body string, tags, images []string) error {
	return s.mutate(slug, func(idea *Idea) error {
		if title != "" {
			idea.Title = title
			idea.Slug = Slugify(title)
		}
		idea.Body = body
		idea.Tags = tags
		idea.Images = images
		// Fallback: extract title from body heading when no explicit title given.
		if title == "" {
			for line := range strings.SplitSeq(body, "\n") {
				if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
					idea.Title = t
					idea.Slug = Slugify(t)
					break
				}
			}
		}
		return nil
	})
}

// Delete soft-deletes an idea by setting its DeletedAt timestamp.
func (s *Service) Delete(slug string) error {
	return s.mutate(slug, func(idea *Idea) error {
		idea.DeletedAt = time.Now().Format("2006-01-02")
		return nil
	})
}

// Restore clears the DeletedAt field, returning an idea from trash.
func (s *Service) Restore(slug string) error {
	return s.mutate(slug, func(idea *Idea) error {
		idea.DeletedAt = ""
		return nil
	})
}

// PermanentDelete removes an idea from the file entirely.
func (s *Service) PermanentDelete(slug string) error {
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

// PurgeExpired permanently removes ideas deleted more than `days` ago.
func (s *Service) PurgeExpired(days int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days)
	var kept []Idea
	for _, idea := range ideas {
		if idea.DeletedAt != "" {
			deletedTime, err := time.Parse("2006-01-02", idea.DeletedAt)
			if err != nil {
				slog.Warn("malformed deleted date, skipping purge for idea", "slug", idea.Slug, "deleted_at", idea.DeletedAt)
				kept = append(kept, idea)
				continue
			}
			if !deletedTime.After(cutoff) {
				continue // purge this idea (deleted on or before cutoff date)
			}
		}
		kept = append(kept, idea)
	}

	if len(kept) == len(ideas) {
		return nil // nothing to purge
	}

	if err := WriteIdeas(s.ideasPath, "Ideas", kept); err != nil {
		return err
	}
	s.cache = kept
	return nil
}

// mutateBatch acquires the lock once, parses the file once, applies fn to all
// matched slugs, writes once, and updates cache once. If any slug is not found,
// the entire batch fails with no changes written.
func (s *Service) mutateBatch(slugs []string, fn func(*Idea) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	slugSet := make(map[string]bool, len(slugs))
	for _, sl := range slugs {
		slugSet[sl] = true
	}

	found := 0
	for i := range ideas {
		if slugSet[ideas[i].Slug] {
			if err := fn(&ideas[i]); err != nil {
				return err
			}
			found++
		}
	}
	if found != len(slugSet) {
		return fmt.Errorf("one or more ideas not found")
	}

	if err := WriteIdeas(s.ideasPath, "Ideas", ideas); err != nil {
		return err
	}
	s.cache = ideas
	return nil
}

// BulkDelete soft-deletes multiple ideas in a single file write.
func (s *Service) BulkDelete(slugs []string) error {
	now := time.Now().Format("2006-01-02")
	return s.mutateBatch(slugs, func(idea *Idea) error {
		idea.DeletedAt = now
		return nil
	})
}

// BulkTriage changes the status of multiple ideas in a single file write.
func (s *Service) BulkTriage(slugs []string, action string) error {
	var status string
	switch action {
	case "park":
		status = "parked"
	case "drop":
		status = "dropped"
	default:
		return fmt.Errorf("unknown bulk triage action %q", action)
	}
	return s.mutateBatch(slugs, func(idea *Idea) error {
		idea.Status = status
		return nil
	})
}

// MarkConverted sets an idea's status to "converted" and records the task slug.
func (s *Service) MarkConverted(slug, taskSlug string) error {
	return s.mutate(slug, func(idea *Idea) error {
		idea.Status = "converted"
		idea.ConvertedTo = taskSlug
		return nil
	})
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

// Search returns ideas whose title or body contains the query (case-insensitive).
// Soft-deleted ideas are excluded from search results.
func (s *Service) Search(query string) []Idea {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var results []Idea
	for _, idea := range s.cache {
		if idea.DeletedAt != "" {
			continue
		}
		if strings.Contains(strings.ToLower(idea.Title), q) || strings.Contains(strings.ToLower(idea.Body), q) {
			results = append(results, idea)
		}
	}
	return results
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
