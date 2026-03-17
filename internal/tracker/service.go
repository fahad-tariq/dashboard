package tracker

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Service struct {
	trackerPath string
	heading     string
	store       *Store
	mu          sync.RWMutex
	cache       []Item
}

func NewService(trackerPath, heading string, store *Store) *Service {
	s := &Service{trackerPath: trackerPath, heading: heading, store: store}
	s.loadCache()
	return s
}

func (s *Service) loadCache() {
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		items = nil
	}
	s.cache = items
}

func (s *Service) mutate(slug string, fn func(*Item) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			if err := fn(&items[i]); err != nil {
				return err
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, s.heading, items); err != nil {
		return err
	}
	s.cache = items
	return s.store.ReplaceAll(items)
}

func (s *Service) List() ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Item, len(s.cache))
	copy(out, s.cache)
	return out, nil
}

func (s *Service) Get(slug string) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.cache {
		if s.cache[i].Slug == slug {
			cp := s.cache[i]
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("tracker item %q not found", slug)
}

func (s *Service) AddItem(item Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item.Title == "" {
		return fmt.Errorf("empty title")
	}
	item.Slug = Slugify(item.Title)
	if item.Added == "" {
		item.Added = time.Now().Format("2006-01-02")
	}

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	items = append(items, item)
	if err := WriteTracker(s.trackerPath, s.heading, items); err != nil {
		return err
	}
	s.cache = items
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdateNotes(slug, body string) error {
	return s.mutate(slug, func(it *Item) error {
		it.Body = body
		return nil
	})
}

func (s *Service) Complete(slug string) error {
	return s.mutate(slug, func(it *Item) error {
		it.Done = true
		it.Completed = time.Now().Format("2006-01-02")
		return nil
	})
}

func (s *Service) Uncomplete(slug string) error {
	return s.mutate(slug, func(it *Item) error {
		it.Done = false
		it.Completed = ""
		return nil
	})
}

func (s *Service) Delete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	idx := -1
	for i := range items {
		if items[i].Slug == slug {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	items = append(items[:idx], items[idx+1:]...)
	if err := WriteTracker(s.trackerPath, s.heading, items); err != nil {
		return err
	}
	s.cache = items
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdatePriority(slug, priority string) error {
	return s.mutate(slug, func(it *Item) error {
		it.Priority = priority
		return nil
	})
}

func (s *Service) UpdateTags(slug string, tags []string) error {
	return s.mutate(slug, func(it *Item) error {
		it.Tags = tags
		return nil
	})
}

func (s *Service) UpdateEdit(slug, title, body string, tags, images []string) error {
	return s.mutate(slug, func(it *Item) error {
		if title != "" {
			it.Title = title
			it.Slug = Slugify(title)
		}
		it.Body = body
		it.Tags = tags
		it.Images = images
		return nil
	})
}

func (s *Service) SetProgress(slug string, value float64) error {
	return s.mutate(slug, func(it *Item) error {
		if it.Type != GoalType {
			return fmt.Errorf("goal %q not found", slug)
		}
		it.Current = value
		if it.Current < 0 {
			it.Current = 0
		}
		return nil
	})
}

func (s *Service) UpdateProgress(slug string, delta float64) error {
	return s.mutate(slug, func(it *Item) error {
		if it.Type != GoalType {
			return fmt.Errorf("goal %q not found", slug)
		}
		it.Current += delta
		if it.Current < 0 {
			it.Current = 0
		}
		return nil
	})
}

func (s *Service) Resync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}
	s.cache = items
	return s.store.ReplaceAll(items)
}

// Search returns items whose title or body contains the query (case-insensitive).
func (s *Service) Search(query string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var results []Item
	for _, it := range s.cache {
		if strings.Contains(strings.ToLower(it.Title), q) || strings.Contains(strings.ToLower(it.Body), q) {
			results = append(results, it)
		}
	}
	return results
}

func (s *Service) Summary() (Summary, error) {
	return s.store.Summary()
}
