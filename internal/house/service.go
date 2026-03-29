package house

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fahad/dashboard/internal/httputil"
)

// Service manages maintenance items stored in a single flat-file.
type Service struct {
	maintPath string
	loc       *time.Location
	mu        sync.RWMutex
	cache     []MaintenanceItem
}

// NewService creates a maintenance service operating on the given file path.
func NewService(maintPath string, loc *time.Location) *Service {
	s := &Service{maintPath: maintPath, loc: loc}
	s.loadCache()
	return s
}

func (s *Service) loadCache() {
	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		items = nil
	}
	s.cache = items
}

// List returns all non-deleted maintenance items from cache.
func (s *Service) List() ([]MaintenanceItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []MaintenanceItem
	for _, it := range s.cache {
		if it.DeletedAt == "" {
			out = append(out, it)
		}
	}
	return out, nil
}

// ListDeleted returns only soft-deleted items from cache.
func (s *Service) ListDeleted() []MaintenanceItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []MaintenanceItem
	for _, it := range s.cache {
		if it.DeletedAt != "" {
			out = append(out, it)
		}
	}
	return out
}

// ListOverdue returns non-deleted maintenance items that are overdue.
func (s *Service) ListOverdue(now time.Time) []MaintenanceItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []MaintenanceItem
	for _, it := range s.cache {
		if it.DeletedAt == "" && it.IsOverdue(now, s.loc) {
			out = append(out, it)
		}
	}
	return out
}

// Get returns a single item by slug.
func (s *Service) Get(slug string) (*MaintenanceItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.cache {
		if s.cache[i].Slug == slug {
			cp := s.cache[i]
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("maintenance item %q not found", slug)
}

// Add appends a new maintenance item.
func (s *Service) Add(item *MaintenanceItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item.Added == "" {
		item.Added = time.Now().In(s.loc).Format("2006-01-02")
	}
	item.Slug = Slugify(item.Title)

	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return err
	}

	items = append(items, *item)
	if err := WriteMaintenance(s.maintPath, "Maintenance", items); err != nil {
		return err
	}
	s.cache = items
	return nil
}

func (s *Service) mutate(slug string, fn func(*MaintenanceItem) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return err
	}

	for i := range items {
		if items[i].Slug == slug {
			if err := fn(&items[i]); err != nil {
				return err
			}
			if err := WriteMaintenance(s.maintPath, "Maintenance", items); err != nil {
				return err
			}
			s.cache = items
			return nil
		}
	}
	return fmt.Errorf("maintenance item %q not found", slug)
}

// LogCompletion prepends a new log entry for the given item.
func (s *Service) LogCompletion(slug, note string) error {
	return s.mutate(slug, func(item *MaintenanceItem) error {
		// Strip newlines to prevent markdown injection.
		note = strings.ReplaceAll(note, "\n", " ")
		note = strings.ReplaceAll(note, "\r", " ")
		note = strings.TrimSpace(note)

		entry := LogEntry{
			Date: time.Now().In(s.loc).Format("2006-01-02"),
			Note: note,
		}
		// Prepend (newest first).
		item.Log = append([]LogEntry{entry}, item.Log...)
		return nil
	})
}

// UpdateEdit updates the title and tags of a maintenance item.
func (s *Service) UpdateEdit(slug, title, notes string, tags []string) error {
	return s.mutate(slug, func(item *MaintenanceItem) error {
		if title != "" {
			item.Title = title
			item.Slug = Slugify(title)
		}
		item.Tags = tags
		item.Notes = notes
		return nil
	})
}

// UpdateCadence changes the cadence of a maintenance item.
func (s *Service) UpdateCadence(slug, cadence string) error {
	if _, _, err := ParseCadence(cadence); err != nil {
		return err
	}
	return s.mutate(slug, func(item *MaintenanceItem) error {
		item.Cadence = cadence
		return nil
	})
}

// Delete soft-deletes a maintenance item.
func (s *Service) Delete(slug string) error {
	return s.mutate(slug, func(item *MaintenanceItem) error {
		item.DeletedAt = time.Now().In(s.loc).Format("2006-01-02")
		return nil
	})
}

// Restore clears the DeletedAt field.
func (s *Service) Restore(slug string) error {
	return s.mutate(slug, func(item *MaintenanceItem) error {
		item.DeletedAt = ""
		return nil
	})
}

// PermanentDelete removes an item from the file entirely.
func (s *Service) PermanentDelete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return err
	}

	for i := range items {
		if items[i].Slug == slug {
			items = append(items[:i], items[i+1:]...)
			if err := WriteMaintenance(s.maintPath, "Maintenance", items); err != nil {
				return err
			}
			s.cache = items
			return nil
		}
	}
	return fmt.Errorf("maintenance item %q not found", slug)
}

// PurgeExpired permanently removes items deleted more than `days` ago.
func (s *Service) PurgeExpired(days int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return err
	}

	cutoff := httputil.CutoffDate(days, s.loc)
	var kept []MaintenanceItem
	for _, it := range items {
		if it.DeletedAt != "" {
			deletedTime, err := time.Parse("2006-01-02", it.DeletedAt)
			if err != nil {
				slog.Warn("malformed deleted date, skipping purge", "slug", it.Slug, "deleted_at", it.DeletedAt)
				kept = append(kept, it)
				continue
			}
			if !deletedTime.After(cutoff) {
				continue // purge
			}
		}
		kept = append(kept, it)
	}

	if len(kept) == len(items) {
		return nil
	}

	if err := WriteMaintenance(s.maintPath, "Maintenance", kept); err != nil {
		return err
	}
	s.cache = kept
	return nil
}

// Search returns items whose title matches the query (case-insensitive).
// Soft-deleted items are excluded.
func (s *Service) Search(query string) []MaintenanceItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var results []MaintenanceItem
	for _, it := range s.cache {
		if it.DeletedAt != "" {
			continue
		}
		if strings.Contains(strings.ToLower(it.Title), q) {
			results = append(results, it)
		}
	}
	return results
}

// Resync refreshes the in-memory cache from disk.
func (s *Service) Resync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return err
	}
	s.cache = items
	return nil
}
