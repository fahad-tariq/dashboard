package ideas

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

// Service manages ideas stored in a single flat-file (ideas.md).
// Follows the tracker's read-modify-write pattern: parse, mutate, write back.
type Service struct {
	changes.Recorder

	ideasPath string
	loc       *time.Location
	mu        sync.RWMutex
	// assignedOnLoad is set once by the constructor\'s load.
	assignedOnLoad bool
	cache          []Idea
}

// NewService creates a new ideas service operating on the given ideas.md file path.
func NewService(ideasPath string, loc *time.Location) *Service {
	s := &Service{ideasPath: ideasPath, loc: loc}
	s.loadCache()
	return s
}

func (s *Service) loadCache() {
	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		ideas = nil
	}
	s.cache = ideas
	s.assignedOnLoad = s.recordOrAssign(ideas)
}

// assignIDs gives ideas without an ID (or repeating one) a new ID in place.
func assignIDs(ideas []Idea) bool {
	return itemid.Assign(ideas, func(idea *Idea) *string { return &idea.ID })
}

// recordOrAssign records freshly parsed items as the file's content, or
// assigns their missing IDs and saves them, so the cache and the file hold
// the same IDs, and reports whether it assigned any. Callers hold s.mu or
// own s.
func (s *Service) recordOrAssign(items []Idea) bool {
	if s.assignAndSave(items) {
		return true
	}
	s.recordOnDisk()
	return false
}

// AssignedOnLoad reports whether the service gave items IDs, and saved
// them, when it was created.
func (s *Service) AssignedOnLoad() bool {
	return s.assignedOnLoad
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

// GetByID returns a single idea by ID.
func (s *Service) GetByID(id string) (*Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.cache {
		if s.cache[i].ID == id {
			cp := s.cache[i]
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("idea %q not found", id)
}

// Add appends a new idea to the ideas.md file and sets idea.ID.
func (s *Service) Add(idea *Idea) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idea.Title = httputil.CleanTitle(idea.Title)
	if idea.Title == "" {
		return fmt.Errorf("empty title")
	}
	idea.Slug = Slugify(idea.Title)
	if idea.Added == "" {
		idea.Added = time.Now().In(s.loc).Format("2006-01-02")
	}
	if idea.Status == "" {
		idea.Status = "untriaged"
	}

	ideas, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return err
	}

	taken := func(id string) bool {
		return slices.ContainsFunc(ideas, func(i Idea) bool { return i.ID == id })
	}
	if !itemid.Valid(idea.ID) || taken(idea.ID) {
		idea.ID = itemid.NewUnique(taken)
	}
	ideas = append(ideas, *idea)
	if err := s.write(ideas); err != nil {
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
			if err := s.write(ideas); err != nil {
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
	title = httputil.CleanTitle(title)
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
					if t = httputil.CleanTitle(t); t != "" {
						idea.Title = t
						idea.Slug = Slugify(t)
					}
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
		idea.DeletedAt = time.Now().In(s.loc).Format("2006-01-02")
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
			if err := s.write(ideas); err != nil {
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

	cutoff := httputil.CutoffDate(days, s.loc)
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

	if err := s.write(kept); err != nil {
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

	if err := s.write(ideas); err != nil {
		return err
	}
	s.cache = ideas
	return nil
}

// BulkDelete soft-deletes multiple ideas in a single file write.
func (s *Service) BulkDelete(slugs []string) error {
	now := time.Now().In(s.loc).Format("2006-01-02")
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

// MarkConverted sets an idea's status to "converted" and records the task's ID.
func (s *Service) MarkConverted(slug, taskID string) error {
	return s.mutate(slug, func(idea *Idea) error {
		idea.Status = "converted"
		idea.ConvertedTo = taskID
		return nil
	})
}

// RelinkConvertedTo rewrites each [converted-to:] value for which resolve returns
// a replacement (an older file's task slug becoming the task's ID) and
// reports how many changed.
func (s *Service) RelinkConvertedTo(resolve func(ref string) (string, bool)) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range items {
		if ref := items[i].ConvertedTo; ref != "" {
			if id, ok := resolve(ref); ok && id != ref {
				items[i].ConvertedTo = id
				n++
			}
		}
	}
	if n == 0 {
		return 0, nil
	}
	if err := s.write(items); err != nil {
		return 0, err
	}
	s.cache = items
	return n, nil
}

// All returns every cached idea, soft-deleted ones included.
func (s *Service) All() []Idea {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cache)
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
	s.recordOrAssign(ideas)
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

// write saves items, then records and publishes the change. Callers hold
// s.mu.
func (s *Service) write(items []Idea) error {
	data, err := s.save(items)
	if err != nil {
		return err
	}
	s.Wrote(data)
	return nil
}

// save assigns missing IDs in place, renders the file and replaces it
// atomically. Callers hold s.mu.
func (s *Service) save(items []Idea) ([]byte, error) {
	assignIDs(items)
	data := RenderIdeas("Ideas", items)
	if err := atomicfile.Write(s.ideasPath, data, 0o644); err != nil {
		return nil, err
	}
	return data, nil
}

// assignAndSave gives freshly parsed items any missing IDs and, if it did,
// saves the file and records it without publishing: the load or watcher
// event that parsed them already tells open pages. Callers hold s.mu or own
// s.
func (s *Service) assignAndSave(items []Idea) bool {
	before := make([]string, len(items))
	for i := range items {
		before[i] = items[i].ID
	}
	if !assignIDs(items) {
		return false
	}
	data, err := s.save(items)
	if err != nil {
		// The file keeps its old IDs, so the cache must too.
		for i := range items {
			items[i].ID = before[i]
		}
		slog.Error("writing assigned item ids", "file", s.ideasPath, "error", err)
		return false
	}
	s.Record(data)
	return true
}

// recordOnDisk notes the file's current content so a watcher event for it is
// not mistaken for an external edit.
func (s *Service) recordOnDisk() {
	if data, err := os.ReadFile(s.ideasPath); err == nil {
		s.Differs(data)
	}
}

// ResyncIfChanged re-reads the file only if it differs from what this service
// last wrote or read, and reports whether it did. The watcher calls it for
// every event, so the service's own writes cost nothing.
func (s *Service) ResyncIfChanged() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.ideasPath)
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
	parsed, err := ParseIdeas(s.ideasPath)
	if err != nil {
		return false, err
	}
	s.cache = parsed
	// An external edit may add ideas without IDs. Saving them back is
	// recorded, so the watcher event it causes is skipped.
	s.assignAndSave(parsed)
	return true, nil
}
