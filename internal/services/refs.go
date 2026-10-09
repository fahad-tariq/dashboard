package services

import (
	"log/slog"

	"github.com/fahad/dashboard/internal/tracker"
)

// refIndex resolves the value of a [from-idea:] or [converted-to:] tag
// against one side's items. Older files hold slugs there; a slug naming
// exactly one item becomes that item's ID.
type refIndex struct {
	ids    map[string]bool
	bySlug map[string]string
	shared map[string]bool // slugs held by more than one item
}

func newRefIndex() *refIndex {
	return &refIndex{ids: map[string]bool{}, bySlug: map[string]string{}, shared: map[string]bool{}}
}

func (x *refIndex) add(id, slug string) {
	x.ids[id] = true
	if _, ok := x.bySlug[slug]; ok {
		x.shared[slug] = true
	}
	x.bySlug[slug] = id
}

// resolver returns the resolve function the services' Relink methods take,
// counting references it can neither keep nor resolve in unresolved.
func (x *refIndex) resolver(unresolved *int) func(string) (string, bool) {
	return func(ref string) (string, bool) {
		if x.ids[ref] {
			return ref, false
		}
		if id, ok := x.bySlug[ref]; ok && !x.shared[ref] {
			return id, true
		}
		*unresolved++
		return ref, false
	}
}

// linkReferences turns the slugs older files hold in [from-idea:] and
// [converted-to:] into IDs, for one user's ideas and the task lists a
// conversion can target: the user's own, family and house. Ambiguous or
// missing targets stay as they are (dangling references are accepted). It
// reads one service at a time and never holds two service locks.
func (r *Registry) linkReferences(userID int64, u *UserServices) {
	lists := []*tracker.Service{u.Personal, r.familySvc, r.houseProjectsSvc}

	ideaIdx := newRefIndex()
	for _, idea := range u.Ideas.All() {
		ideaIdx.add(idea.ID, idea.Slug)
	}
	unresolvedIdeas := 0
	resolveIdea := ideaIdx.resolver(&unresolvedIdeas)
	for _, l := range lists {
		if n, err := l.RelinkFromIdea(resolveIdea); err != nil {
			slog.Error("linking from-idea references", "user_id", userID, "error", err)
		} else if n > 0 {
			slog.Info("from-idea references now hold IDs", "user_id", userID, "count", n)
		}
	}

	taskIdx := newRefIndex()
	for _, l := range lists {
		for _, it := range l.All() {
			taskIdx.add(it.ID, it.Slug)
		}
	}
	unresolvedTasks := 0
	if n, err := u.Ideas.RelinkConvertedTo(taskIdx.resolver(&unresolvedTasks)); err != nil {
		slog.Error("linking converted-to references", "user_id", userID, "error", err)
	} else if n > 0 {
		slog.Info("converted-to references now hold IDs", "user_id", userID, "count", n)
	}
	if unresolvedIdeas+unresolvedTasks > 0 {
		slog.Info("references left as they were: ambiguous, missing, or another user's",
			"user_id", userID, "from_idea", unresolvedIdeas, "converted_to", unresolvedTasks)
	}
}
