package house

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fahad/dashboard/internal/atomicfile"
	"github.com/fahad/dashboard/internal/changes"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/itemid"
)

// Service manages maintenance items stored in a single flat-file.
type Service struct {
	changes.Recorder

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
	s.recordOrAssign(items)
}

// assignIDs gives items without an ID (or repeating one) a new ID in place.
func assignIDs(items []MaintenanceItem) bool {
	return itemid.Assign(items, func(it *MaintenanceItem) *string { return &it.ID })
}

// recordOrAssign records freshly parsed items as the file's content, or
// assigns their missing IDs and saves them, so the cache and the file hold
// the same IDs. Callers hold s.mu or own s.
func (s *Service) recordOrAssign(items []MaintenanceItem) {
	if !s.assignAndSave(items) {
		s.recordOnDisk()
	}
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

	item.Title = httputil.CleanTitle(item.Title)
	if item.Title == "" {
		return fmt.Errorf("empty title")
	}
	if item.Added == "" {
		item.Added = time.Now().In(s.loc).Format("2006-01-02")
	}
	item.Slug = Slugify(item.Title)

	items, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return err
	}

	taken := func(id string) bool {
		return slices.ContainsFunc(items, func(it MaintenanceItem) bool { return it.ID == id })
	}
	if !itemid.Valid(item.ID) || taken(item.ID) {
		item.ID = itemid.NewUnique(taken)
	}
	items = append(items, *item)
	if err := s.write(items); err != nil {
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
			if err := s.write(items); err != nil {
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
		if title := httputil.CleanTitle(title); title != "" {
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
			if err := s.write(items); err != nil {
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

	if err := s.write(kept); err != nil {
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
	s.recordOrAssign(items)
	return nil
}

// write saves items, then records and publishes the change. Callers hold
// s.mu.
func (s *Service) write(items []MaintenanceItem) error {
	data, err := s.save(items)
	if err != nil {
		return err
	}
	s.Wrote(data)
	return nil
}

// save assigns missing IDs in place, renders the file and replaces it
// atomically. Callers hold s.mu.
func (s *Service) save(items []MaintenanceItem) ([]byte, error) {
	assignIDs(items)
	data := RenderMaintenance("Maintenance", items)
	if err := atomicfile.Write(s.maintPath, data, 0o644); err != nil {
		return nil, err
	}
	return data, nil
}

// assignAndSave gives freshly parsed items any missing IDs and, if it did,
// saves the file and records it without publishing: the load or watcher
// event that parsed them already tells open pages. Callers hold s.mu or own
// s.
func (s *Service) assignAndSave(items []MaintenanceItem) bool {
	if !assignIDs(items) {
		return false
	}
	data, err := s.save(items)
	if err != nil {
		slog.Error("writing assigned item ids", "file", s.maintPath, "error", err)
		return false
	}
	s.Record(data)
	return true
}

// recordOnDisk notes the file's current content so a watcher event for it is
// not mistaken for an external edit.
func (s *Service) recordOnDisk() {
	if data, err := os.ReadFile(s.maintPath); err == nil {
		s.Differs(data)
	}
}

// ResyncIfChanged re-reads the file only if it differs from what this service
// last wrote or read, and reports whether it did. The watcher calls it for
// every event, so the service's own writes cost nothing.
func (s *Service) ResyncIfChanged() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.maintPath)
	if errors.Is(err, fs.ErrNotExist) {
		// Deleted outside the app: the list is now empty.
		if !s.Differs(nil) {
			return false, nil
		}
		s.cache = nil
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !s.Differs(data) {
		return false, nil
	}
	parsed, err := ParseMaintenance(s.maintPath)
	if err != nil {
		return false, err
	}
	s.cache = parsed
	// An external edit may add items without IDs. Saving them back is
	// recorded, so the watcher event it causes is skipped.
	s.assignAndSave(parsed)
	return true, nil
}
