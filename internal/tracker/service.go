package tracker

import (
	"fmt"
	"sync"
	"time"
)

type Service struct {
	trackerPath string
	store       *Store
	mu          sync.RWMutex
}

func NewService(trackerPath string, store *Store) *Service {
	return &Service{trackerPath: trackerPath, store: store}
}

func (s *Service) List() ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ParseTracker(s.trackerPath)
}

func (s *Service) Get(slug string) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Slug == slug {
			return &items[i], nil
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
	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdateNotes(slug, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			items[i].Body = body
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) Complete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			items[i].Done = true
			items[i].Completed = time.Now().Format("2006-01-02")
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) Uncomplete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			items[i].Done = false
			items[i].Completed = ""
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
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
	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdatePriority(slug, priority string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			items[i].Priority = priority
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdateTags(slug string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			items[i].Tags = tags
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdateEdit(slug, body string, tags, images []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug {
			items[i].Body = body
			items[i].Tags = tags
			items[i].Images = images
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) SetProgress(slug string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug && items[i].Type == GoalType {
			items[i].Current = value
			if items[i].Current < 0 {
				items[i].Current = 0
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("goal %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) UpdateProgress(slug string, delta float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if items[i].Slug == slug && items[i].Type == GoalType {
			items[i].Current += delta
			if items[i].Current < 0 {
				items[i].Current = 0
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("goal %q not found", slug)
	}

	if err := WriteTracker(s.trackerPath, items); err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) Resync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

func (s *Service) Summary() (Summary, error) {
	return s.store.Summary()
}
