package tracker

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fahad/dashboard/internal/httputil"
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
	return s.store.ReplaceAll(activeItems(items))
}

// activeItems returns only non-deleted items for DB cache sync.
func activeItems(items []Item) []Item {
	var out []Item
	for _, it := range items {
		if it.DeletedAt == "" {
			out = append(out, it)
		}
	}
	return out
}

func (s *Service) List() ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	for _, it := range s.cache {
		if it.DeletedAt == "" {
			out = append(out, it)
		}
	}
	return out, nil
}

// ListDeleted returns only soft-deleted items from the cache.
func (s *Service) ListDeleted() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	for _, it := range s.cache {
		if it.DeletedAt != "" {
			out = append(out, it)
		}
	}
	return out
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
	return s.store.ReplaceAll(activeItems(items))
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

// Delete soft-deletes an item by setting its DeletedAt timestamp.
func (s *Service) Delete(slug string) error {
	return s.mutate(slug, func(it *Item) error {
		it.DeletedAt = time.Now().Format("2006-01-02")
		return nil
	})
}

// Restore clears the DeletedAt field, returning an item from trash.
func (s *Service) Restore(slug string) error {
	return s.mutate(slug, func(it *Item) error {
		it.DeletedAt = ""
		return nil
	})
}

// PermanentDelete removes an item from the file entirely.
func (s *Service) PermanentDelete(slug string) error {
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
	return s.store.ReplaceAll(activeItems(items))
}

// PurgeExpired permanently removes items deleted more than `days` ago.
func (s *Service) PurgeExpired(days int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	cutoff := httputil.CutoffDate(days)
	var kept []Item
	for _, it := range items {
		if it.DeletedAt != "" {
			deletedTime, err := time.Parse("2006-01-02", it.DeletedAt)
			if err != nil {
				slog.Warn("malformed deleted date, skipping purge for item", "slug", it.Slug, "deleted_at", it.DeletedAt)
				kept = append(kept, it)
				continue
			}
			if !deletedTime.After(cutoff) {
				continue // purge this item (deleted on or before cutoff date)
			}
		}
		kept = append(kept, it)
	}

	if len(kept) == len(items) {
		return nil // nothing to purge
	}

	if err := WriteTracker(s.trackerPath, s.heading, kept); err != nil {
		return err
	}
	s.cache = kept
	return s.store.ReplaceAll(activeItems(kept))
}

// mutateBatch acquires the lock once, parses the file once, applies fn to all
// matched slugs, writes once, and updates cache once. If any slug is not found,
// the entire batch fails with no changes written.
func (s *Service) mutateBatch(slugs []string, fn func(*Item) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	slugSet := make(map[string]bool, len(slugs))
	for _, sl := range slugs {
		slugSet[sl] = true
	}

	found := 0
	for i := range items {
		if slugSet[items[i].Slug] {
			if err := fn(&items[i]); err != nil {
				return err
			}
			found++
		}
	}
	if found != len(slugSet) {
		return fmt.Errorf("one or more tracker items not found")
	}

	if err := WriteTracker(s.trackerPath, s.heading, items); err != nil {
		return err
	}
	s.cache = items
	return s.store.ReplaceAll(activeItems(items))
}

// BulkComplete marks multiple items as done in a single file write.
func (s *Service) BulkComplete(slugs []string) error {
	now := time.Now().Format("2006-01-02")
	return s.mutateBatch(slugs, func(it *Item) error {
		it.Done = true
		it.Completed = now
		return nil
	})
}

// BulkDelete soft-deletes multiple items in a single file write.
func (s *Service) BulkDelete(slugs []string) error {
	now := time.Now().Format("2006-01-02")
	return s.mutateBatch(slugs, func(it *Item) error {
		it.DeletedAt = now
		return nil
	})
}

// BulkUpdatePriority sets the priority on multiple items in a single file write.
func (s *Service) BulkUpdatePriority(slugs []string, priority string) error {
	return s.mutateBatch(slugs, func(it *Item) error {
		it.Priority = priority
		return nil
	})
}

// BulkAddTag appends a tag to multiple items in a single file write.
// Skips items that already have the tag.
func (s *Service) BulkAddTag(slugs []string, tag string) error {
	return s.mutateBatch(slugs, func(it *Item) error {
		for _, t := range it.Tags {
			if t == tag {
				return nil
			}
		}
		it.Tags = append(it.Tags, tag)
		return nil
	})
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
	return s.store.ReplaceAll(activeItems(items))
}

// Search returns items whose title or body contains the query (case-insensitive).
// Soft-deleted items are excluded from search results.
func (s *Service) Search(query string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var results []Item
	for _, it := range s.cache {
		if it.DeletedAt != "" {
			continue
		}
		if strings.Contains(strings.ToLower(it.Title), q) || strings.Contains(strings.ToLower(it.Body), q) {
			results = append(results, it)
		}
	}
	return results
}

func (s *Service) Summary() (Summary, error) {
	return s.store.Summary()
}
