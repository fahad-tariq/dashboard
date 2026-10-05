package tasks

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/tracker"
)

// FamilyID is the family module's ID.
const FamilyID = "family"

// Family is the task list every user shares, in FAMILY_PATH.
type Family struct {
	deps    module.Deps
	handler *tracker.Handler
}

// NewFamily builds the family module.
func NewFamily(deps module.Deps) module.Module {
	m := &Family{deps: deps}
	m.handler = tracker.NewHandlerWithResolver(m.lists, deps.Render.Lookup(templates), FamilyID, deps.Location)
	m.handler.SetCommentaryStore(deps.Commentary)
	return m
}

// lists returns family and the request user's own list, where "move" sends
// items.
func (m *Family) lists(r *http.Request) (*tracker.Service, *tracker.Service) {
	return m.deps.Services.Family(), m.deps.Services.ForUser(auth.UserID(r.Context())).Personal
}

func (m *Family) family(int64) *tracker.Service { return m.deps.Services.Family() }

func (m *Family) Manifest() module.Manifest {
	return module.Manifest{
		ID:        FamilyID,
		Title:     "Family",
		Nav:       []module.NavItem{{Label: "family", Path: "/family", Order: 50, Group: module.Primary, Shortcut: "f"}},
		Prefixes:  []string{"/family"},
		Templates: templates,
	}
}

func (m *Family) Routes(r chi.Router) {
	r.Get("/family", m.handler.TrackerPage)
	m.handler.Mount(r, "/family")
}

func (m *Family) Watches() []module.WatchSpec {
	return []module.WatchSpec{{Path: m.deps.Config.FamilyPath, Reload: resync(m.deps.Services.Family())}}
}

func (m *Family) Search(ctx context.Context, userID int64, q string) []module.SearchResult {
	return search.Tracker(FamilyID, "/family#item-", m.family).Search(ctx, userID, q)
}

func (m *Family) Widgets(_ context.Context, _ int64, now time.Time) []module.WidgetData {
	if w, ok := taskWidget("", "Family Tasks", "/family", m.deps.Services.Family(), now.Format("2006-01-02")); ok {
		return []module.WidgetData{w}
	}
	return nil
}
