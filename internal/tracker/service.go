package tracker

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

// ErrInvalidDate is returned for a date that is not a real YYYY-MM-DD day.
var ErrInvalidDate = errors.New("invalid date")

// validDate rejects anything but a real YYYY-MM-DD day. Dates are written
// into the item line as given, so this also stops a date injecting a tag.
func validDate(date string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ErrInvalidDate
	}
	return nil
}

type Service struct {
	changes.Recorder

	trackerPath string
	heading     string
	loc         *time.Location
	mu          sync.RWMutex
	// assignedOnLoad is set once by the constructor's load.
	assignedOnLoad bool
	cache          []Item
}

func NewService(trackerPath, heading string, loc *time.Location) *Service {
	s := &Service{trackerPath: trackerPath, heading: heading, loc: loc}
	s.loadCache()
	return s
}

func (s *Service) loadCache() {
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		items = nil
	}
	s.cache = items
	s.assignedOnLoad = s.recordOrAssign(items)
}

// assignIDs gives items without an ID (or repeating one) a new ID in place.
func assignIDs(items []Item) bool {
	return itemid.Assign(items, func(it *Item) *string { return &it.ID })
}

// recordOrAssign records freshly parsed items as the file's content, or
// assigns their missing IDs and saves them, so the cache and the file hold
// the same IDs, and reports whether it assigned any. Callers hold s.mu or
// own s.
func (s *Service) recordOrAssign(items []Item) bool {
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

func (s *Service) mutate(id string, fn func(*Item) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	found := false
	for i := range items {
		if id != "" && items[i].ID == id {
			if err := fn(&items[i]); err != nil {
				return err
			}
			items[i].SubStepsDone, items[i].SubStepsTotal = countSubSteps(items[i].Body)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tracker item %q not found", id)
	}

	if err := s.write(items); err != nil {
		return err
	}
	s.cache = items
	return nil
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

func (s *Service) Get(id string) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.cache {
		if id != "" && s.cache[i].ID == id {
			cp := s.cache[i]
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("tracker item %q not found", id)
}

// AddItem appends item and returns its ID. An item keeps an ID it brings
// (a move) unless this list already holds it.
func (s *Service) AddItem(item Item) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item.Title = httputil.CleanTitle(item.Title)
	if item.Title == "" {
		return "", fmt.Errorf("empty title")
	}
	for _, date := range []string{item.Deadline, item.Planned} {
		if date == "" {
			continue
		}
		if err := validDate(date); err != nil {
			return "", err
		}
	}
	if item.Added == "" {
		item.Added = time.Now().In(s.loc).Format("2006-01-02")
	}

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return "", err
	}

	taken := func(id string) bool {
		return slices.ContainsFunc(items, func(it Item) bool { return it.ID == id })
	}
	if !itemid.Valid(item.ID) || taken(item.ID) {
		item.ID = itemid.NewUnique(taken)
	}
	items = append(items, item)
	if err := s.write(items); err != nil {
		return "", err
	}
	s.cache = items
	return item.ID, nil
}

func (s *Service) UpdateNotes(id, body string) error {
	return s.mutate(id, func(it *Item) error {
		it.Body = body
		return nil
	})
}

func (s *Service) Complete(id string) error {
	now := time.Now().In(s.loc).Format("2006-01-02")
	return s.mutate(id, func(it *Item) error {
		markDone(it, now)
		return nil
	})
}

func (s *Service) Uncomplete(id string) error {
	return s.mutate(id, func(it *Item) error {
		it.Done = false
		it.Completed = ""
		// The status before "done" is not stored, so a reopened project
		// starts again from todo.
		if it.Status == "done" {
			it.Status = "todo"
		}
		return nil
	})
}

// markDone completes an item. Only house projects carry a status; theirs
// becomes "done" too, so the house page agrees with the digest.
func markDone(it *Item, today string) {
	it.Done = true
	it.Completed = today
	if it.Status != "" {
		it.Status = "done"
	}
}

// UpdateStatus sets the status field of an item (house projects only).
// Setting status to "done" also marks the item as completed; other statuses clear it.
func (s *Service) UpdateStatus(id, status string) error {
	return s.mutate(id, func(it *Item) error {
		it.Status = status
		if status == "done" {
			it.Done = true
			if it.Completed == "" {
				it.Completed = time.Now().In(s.loc).Format("2006-01-02")
			}
		} else {
			it.Done = false
			it.Completed = ""
		}
		return nil
	})
}

// Delete soft-deletes an item by setting its DeletedAt timestamp.
func (s *Service) Delete(id string) error {
	return s.mutate(id, func(it *Item) error {
		it.DeletedAt = time.Now().In(s.loc).Format("2006-01-02")
		return nil
	})
}

// Restore clears the DeletedAt field, returning an item from trash.
func (s *Service) Restore(id string) error {
	return s.mutate(id, func(it *Item) error {
		it.DeletedAt = ""
		return nil
	})
}

// PermanentDelete removes an item from the file entirely.
func (s *Service) PermanentDelete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	idx := -1
	for i := range items {
		if id != "" && items[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("tracker item %q not found", id)
	}

	items = append(items[:idx], items[idx+1:]...)
	if err := s.write(items); err != nil {
		return err
	}
	s.cache = items
	return nil
}

// PurgeExpired permanently removes items deleted more than `days` ago
// and returns their IDs.
func (s *Service) PurgeExpired(days int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return nil, err
	}

	var purged []string
	cutoff := httputil.CutoffDate(days, s.loc)
	var kept []Item
	for _, it := range items {
		if it.DeletedAt != "" {
			deletedTime, err := time.Parse("2006-01-02", it.DeletedAt)
			if err != nil {
				slog.Warn("malformed deleted date, skipping purge for item", "id", it.ID, "deleted_at", it.DeletedAt)
				kept = append(kept, it)
				continue
			}
			if !deletedTime.After(cutoff) {
				purged = append(purged, it.ID)
				continue // purge this item (deleted on or before cutoff date)
			}
		}
		kept = append(kept, it)
	}

	if len(kept) == len(items) {
		return nil, nil // nothing to purge
	}

	if err := s.write(kept); err != nil {
		return nil, err
	}
	s.cache = kept
	return purged, nil
}

// mutateBatch acquires the lock once, parses the file once, applies fn to all
// matched ids, writes once, and updates cache once. If any id is not found,
// the entire batch fails with no changes written.
func (s *Service) mutateBatch(ids []string, fn func(*Item) error) error {
	if !itemid.AllValid(ids) {
		return fmt.Errorf("one or more tracker items not found")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return err
	}

	idSet := make(map[string]bool, len(ids))
	for _, v := range ids {
		idSet[v] = true
	}

	found := 0
	for i := range items {
		if idSet[items[i].ID] {
			if err := fn(&items[i]); err != nil {
				return err
			}
			items[i].SubStepsDone, items[i].SubStepsTotal = countSubSteps(items[i].Body)
			found++
		}
	}
	if found != len(idSet) {
		return fmt.Errorf("one or more tracker items not found")
	}

	if err := s.write(items); err != nil {
		return err
	}
	s.cache = items
	return nil
}

// BulkComplete marks multiple items as done in a single file write.
func (s *Service) BulkComplete(ids []string) error {
	now := time.Now().In(s.loc).Format("2006-01-02")
	return s.mutateBatch(ids, func(it *Item) error {
		markDone(it, now)
		return nil
	})
}

// BulkDelete soft-deletes multiple items in a single file write.
func (s *Service) BulkDelete(ids []string) error {
	now := time.Now().In(s.loc).Format("2006-01-02")
	return s.mutateBatch(ids, func(it *Item) error {
		it.DeletedAt = now
		return nil
	})
}

// BulkUpdatePriority sets the priority on multiple items in a single file write.
func (s *Service) BulkUpdatePriority(ids []string, priority string) error {
	return s.mutateBatch(ids, func(it *Item) error {
		it.Priority = priority
		return nil
	})
}

// BulkAddTag appends a tag to multiple items in a single file write.
// Skips items that already have the tag.
func (s *Service) BulkAddTag(ids []string, tag string) error {
	return s.mutateBatch(ids, func(it *Item) error {
		if slices.Contains(it.Tags, tag) {
			return nil
		}
		it.Tags = append(it.Tags, tag)
		return nil
	})
}

func (s *Service) UpdatePriority(id, priority string) error {
	return s.mutate(id, func(it *Item) error {
		it.Priority = priority
		return nil
	})
}

func (s *Service) UpdateTags(id string, tags []string) error {
	return s.mutate(id, func(it *Item) error {
		it.Tags = tags
		return nil
	})
}

// Edit is a partial update for ApplyEdit. An empty Title and nil fields keep
// the item's current values; a non-nil empty slice clears that field.
type Edit struct {
	Title    string
	Body     *string
	Tags     *[]string
	Images   *[]string
	Deadline *string // YYYY-MM-DD; empty clears it
}

// ApplyEdit changes only the fields e sets. Every edit goes through it: the
// web form sets every field it shows, the API and the house page only some.
func (s *Service) ApplyEdit(id string, e Edit) error {
	if e.Deadline != nil && *e.Deadline != "" {
		if err := validDate(*e.Deadline); err != nil {
			return err
		}
	}
	return s.mutate(id, func(it *Item) error {
		if e.Deadline != nil {
			it.Deadline = *e.Deadline
		}
		if title := httputil.CleanTitle(e.Title); title != "" {
			it.Title = title
		}
		if e.Body != nil {
			it.Body = *e.Body
		}
		if e.Tags != nil {
			it.Tags = *e.Tags
		}
		if e.Images != nil {
			it.Images = *e.Images
		}
		return nil
	})
}

// RelinkFromIdea rewrites each [from-idea:] value for which resolve returns
// a replacement (an older file's idea id becoming the idea's ID) and
// reports how many changed.
func (s *Service) RelinkFromIdea(resolve func(ref string) (string, bool)) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := ParseTracker(s.trackerPath)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range items {
		if ref := items[i].FromIdea; ref != "" {
			if id, ok := resolve(ref); ok && id != ref {
				items[i].FromIdea = id
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

// All returns every cached item, soft-deleted ones included.
func (s *Service) All() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cache)
}

func (s *Service) SetProgress(id string, value float64) error {
	return s.mutate(id, func(it *Item) error {
		if it.Type != GoalType {
			return fmt.Errorf("goal %q not found", id)
		}
		it.Current = value
		if it.Current < 0 {
			it.Current = 0
		}
		return nil
	})
}

func (s *Service) UpdateProgress(id string, delta float64) error {
	return s.mutate(id, func(it *Item) error {
		if it.Type != GoalType {
			return fmt.Errorf("goal %q not found", id)
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
	s.recordOrAssign(items)
	return nil
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

// SetPlanned marks an item as planned for the given date (YYYY-MM-DD).
// Resets PlanOrder since a new day has no established order yet.
func (s *Service) SetPlanned(id, date string) error {
	if err := validDate(date); err != nil {
		return err
	}
	return s.mutate(id, func(it *Item) error {
		it.Planned = date
		it.PlanOrder = 0
		return nil
	})
}

// ClearPlanned removes the planned date and resets PlanOrder.
func (s *Service) ClearPlanned(id string) error {
	return s.mutate(id, func(it *Item) error {
		it.Planned = ""
		it.PlanOrder = 0
		return nil
	})
}

// BulkSetPlanned sets the planned date on multiple items in a single file write.
// Resets PlanOrder since bulk-planning from /todos shouldn't carry stale order.
func (s *Service) BulkSetPlanned(ids []string, date string) error {
	if err := validDate(date); err != nil {
		return err
	}
	return s.mutateBatch(ids, func(it *Item) error {
		it.Planned = date
		it.PlanOrder = 0
		return nil
	})
}

// ReorderPlanned sets PlanOrder = position+1 for each id in the given order.
func (s *Service) ReorderPlanned(ids []string) error {
	idIndex := make(map[string]int, len(ids))
	for i, v := range ids {
		idIndex[v] = i + 1
	}
	return s.mutateBatch(ids, func(it *Item) error {
		it.PlanOrder = idIndex[it.ID]
		return nil
	})
}

// ListPlanned returns active non-deleted items planned for the given date.
func (s *Service) ListPlanned(date string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	for _, it := range s.cache {
		if it.DeletedAt == "" && it.Planned == date {
			out = append(out, it)
		}
	}
	return out
}

// ListOverdue returns active incomplete items planned before the given date.
func (s *Service) ListOverdue(beforeDate string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	for _, it := range s.cache {
		if it.DeletedAt == "" && !it.Done && it.Planned != "" && it.Planned < beforeDate {
			out = append(out, it)
		}
	}
	return out
}

// ListPlannedRange returns active non-deleted items planned within [start, end] inclusive.
func (s *Service) ListPlannedRange(start, end string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	for _, it := range s.cache {
		if it.DeletedAt == "" && it.Planned >= start && it.Planned <= end {
			out = append(out, it)
		}
	}
	return out
}

// AddSubStep appends a new unchecked sub-step to the item body.
func (s *Service) AddSubStep(id, text string) error {
	return s.mutate(id, func(it *Item) error {
		line := "- [ ] " + strings.TrimSpace(text)
		if it.Body == "" {
			it.Body = line
		} else {
			it.Body += "\n" + line
		}
		return nil
	})
}

// ToggleSubStep toggles the done state of the Nth sub-step in the body.
func (s *Service) ToggleSubStep(id string, index int) error {
	return s.mutate(id, func(it *Item) error {
		lines := strings.Split(it.Body, "\n")
		stepIdx := 0
		for i, line := range lines {
			if !isSubStepLine(line) {
				continue
			}
			if stepIdx == index {
				switch {
				case strings.HasPrefix(line, "- [ ] "):
					lines[i] = "- [x] " + line[6:]
				case strings.HasPrefix(line, "- [x] "), strings.HasPrefix(line, "- [X] "):
					lines[i] = "- [ ] " + line[6:]
				}
				it.Body = strings.Join(lines, "\n")
				return nil
			}
			stepIdx++
		}
		return fmt.Errorf("sub-step index %d out of range", index)
	})
}

// RemoveSubStep removes the Nth sub-step from the body.
func (s *Service) RemoveSubStep(id string, index int) error {
	return s.mutate(id, func(it *Item) error {
		lines := strings.Split(it.Body, "\n")
		stepIdx := 0
		for i, line := range lines {
			if !isSubStepLine(line) {
				continue
			}
			if stepIdx == index {
				lines = append(lines[:i], lines[i+1:]...)
				it.Body = strings.TrimSpace(strings.Join(lines, "\n"))
				return nil
			}
			stepIdx++
		}
		return fmt.Errorf("sub-step index %d out of range", index)
	})
}

func (s *Service) Summary() (Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sum Summary
	for _, it := range s.cache {
		if it.DeletedAt != "" || it.Done {
			continue
		}
		switch it.Type {
		case TaskType:
			sum.OpenTasks++
		case GoalType:
			sum.ActiveGoals++
		}
	}
	return sum, nil
}

// write saves items, then records and publishes the change. Callers hold
// s.mu.
func (s *Service) write(items []Item) error {
	data, err := s.save(items)
	if err != nil {
		return err
	}
	s.Wrote(data)
	return nil
}

// save assigns missing IDs in place, renders the file and replaces it
// atomically. Callers hold s.mu.
func (s *Service) save(items []Item) ([]byte, error) {
	assignIDs(items)
	data := RenderTracker(s.heading, items)
	if err := atomicfile.Write(s.trackerPath, data, 0o644); err != nil {
		return nil, err
	}
	return data, nil
}

// assignAndSave gives freshly parsed items any missing IDs and, if it did,
// saves the file and records it without publishing: the load or watcher
// event that parsed them already tells open pages. Callers hold s.mu or own
// s.
func (s *Service) assignAndSave(items []Item) bool {
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
		slog.Error("writing assigned item ids", "file", s.trackerPath, "error", err)
		return false
	}
	s.Record(data)
	return true
}

// recordOnDisk notes the file's current content so a watcher event for it is
// not mistaken for an external edit.
func (s *Service) recordOnDisk() {
	if data, err := os.ReadFile(s.trackerPath); err == nil {
		s.Differs(data)
	}
}

// ResyncIfChanged re-reads the file only if it differs from what this service
// last wrote or read, and reports whether it did. The watcher calls it for
// every event, so the service's own writes cost nothing.
func (s *Service) ResyncIfChanged() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.trackerPath)
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
	parsed, err := ParseTracker(s.trackerPath)
	if err != nil {
		return false, err
	}
	s.cache = parsed
	// An external edit may add items without IDs. Saving them back is
	// recorded, so the watcher event it causes is skipped.
	s.assignAndSave(parsed)
	return true, nil
}
