package tracker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Service provides tracker business logic with mutex-protected file access.
type Service struct {
	trackerPath string
	store       *Store
	mu          sync.RWMutex
}

// NewService creates a tracker service.
func NewService(trackerPath string, store *Store) *Service {
	return &Service{trackerPath: trackerPath, store: store}
}

// List returns all items from tracker.md.
func (s *Service) List() ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ParseTracker(s.trackerPath)
}

// Get returns a single item by slug.
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

// AddItem creates a new item (task or goal).
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

// UpdateNotes updates the body text of an item.
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

// Complete marks a task as done.
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

// Uncomplete marks a done task as not done.
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

// Delete removes an item by slug.
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

// UpdatePriority changes an item's priority.
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

// UpdateTags changes an item's additional tags.
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

// SetProgress sets a goal's current value to an absolute number.
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

// UpdateProgress changes a goal's current value by delta.
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

// Graduate creates a project directory from a tracker item.
// Rolls back the created directory on any failure.
func (s *Service) Graduate(slug, projectsDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	var target *Item
	for i := range items {
		if items[i].Slug == slug {
			target = &items[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("tracker item %q not found", slug)
	}
	if target.Graduated {
		return fmt.Errorf("item %q is already graduated", slug)
	}

	safeSlug := Slugify(target.Title)
	if safeSlug == "" {
		return fmt.Errorf("cannot derive a safe directory name from %q", target.Title)
	}

	dir := filepath.Join(projectsDir, safeSlug)

	// Validate no path traversal.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	absProjects, err := filepath.Abs(projectsDir)
	if err != nil {
		return fmt.Errorf("resolving projects dir: %w", err)
	}
	if !isSubdir(absDir, absProjects) {
		return fmt.Errorf("path traversal detected")
	}

	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("directory %q already exists", dir)
	}

	// Create directory -- roll back on any subsequent failure.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	rollback := func() { os.RemoveAll(dir) }

	// git init
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		rollback()
		return fmt.Errorf("git init: %w", err)
	}

	// Write README.md seeded from item.
	readme := fmt.Sprintf("# %s\n\n%s\n", target.Title, target.Body)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		rollback()
		return fmt.Errorf("writing README: %w", err)
	}

	// Mark as graduated and write back.
	target.Graduated = true
	if err := WriteTracker(s.trackerPath, items); err != nil {
		rollback()
		return err
	}

	return s.store.ReplaceAll(items)
}

// Resync re-reads tracker.md and updates the SQLite cache.
func (s *Service) Resync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}
	return s.store.ReplaceAll(items)
}

// Summary returns aggregate counts from the cache.
func (s *Service) Summary() (Summary, error) {
	return s.store.Summary()
}

func isSubdir(child, parent string) bool {
	return child == parent || len(child) > len(parent) &&
		child[:len(parent)] == parent &&
		child[len(parent)] == filepath.Separator
}
