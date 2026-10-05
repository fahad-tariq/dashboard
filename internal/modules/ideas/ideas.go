// Package ideas is the ideas module: capture, triage, research notes and
// conversion to tasks, over each user's ideas.md.
package ideas

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	idealib "github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/tracker"
)

// ID is the module's ID and template directory.
const ID = "ideas"

// untriagedShown is how many untriaged ideas the home widget lists.
const untriagedShown = 3

// Module is the ideas module.
type Module struct {
	deps    module.Deps
	handler *idealib.Handler
}

// New builds the ideas module.
func New(deps module.Deps) module.Module {
	m := &Module{deps: deps}
	m.handler = idealib.NewHandlerWithResolver(func(r *http.Request) *idealib.Service {
		return m.ideas(auth.UserID(r.Context()))
	}, m.toTask, deps.Render.Lookup(ID), deps.Location)
	m.handler.SetCommentaryStore(deps.Commentary)
	return m
}

func (m *Module) ideas(userID int64) *idealib.Service {
	return m.deps.Services.ForUser(userID).Ideas
}

// toTask adds a task converted from an idea to the target list ("personal",
// "family" or "house") and returns its slug.
func (m *Module) toTask(ctx context.Context, title, body string, tags []string, fromIdeaSlug, target string) (string, error) {
	item := tracker.Item{
		Title:    title,
		Type:     tracker.TaskType,
		Body:     body,
		Tags:     tags,
		FromIdea: fromIdeaSlug,
	}
	switch target {
	case "family":
		return m.deps.Services.Family().AddItem(item)
	case "house":
		item.Status = "todo"
		return m.deps.Services.HouseProjects().AddItem(item)
	default:
		return m.deps.Services.ForUser(auth.UserID(ctx)).Personal.AddItem(item)
	}
}

func (m *Module) Manifest() module.Manifest {
	return module.Manifest{
		ID:       ID,
		Title:    "Ideas",
		Nav:      []module.NavItem{{Label: "ideas", Path: "/ideas", Order: 30, Group: module.Primary, Shortcut: "i"}},
		Prefixes: []string{"/ideas", "/exploration"},
	}
}

func (m *Module) Routes(r chi.Router) {
	h := m.handler
	r.Get("/ideas", h.IdeasPage)
	r.Get("/ideas/{slug}", h.IdeaDetail)
	r.Post("/ideas/add", h.QuickAdd)
	r.Post("/ideas/{slug}/triage", h.TriageAction)
	r.Post("/ideas/{slug}/to-task", h.ToTask)
	r.Post("/ideas/{slug}/edit", h.Edit)
	r.Post("/ideas/{slug}/delete", h.DeleteIdea)
	r.Post("/ideas/{slug}/restore", h.RestoreIdea)
	r.Post("/ideas/{slug}/purge", h.PermanentDeleteIdea)
	r.Post("/ideas/bulk/delete", h.BulkDeleteIdeas)
	r.Post("/ideas/bulk/triage", h.BulkTriageIdeas)

	r.Get("/exploration", http.RedirectHandler("/ideas", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/exploration/{slug}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ideas/"+chi.URLParam(r, "slug"), http.StatusMovedPermanently) //nolint:gosec // G710: chi params cannot contain "/", so the target stays under /ideas/
	})
}

func (m *Module) APIRoutes(r chi.Router) {
	r.Get("/ideas", m.handler.APIListIdeas)
	r.Post("/ideas", m.handler.APIAddIdea)
	r.Put("/ideas/{slug}/triage", m.handler.APITriageIdea)
	r.Post("/ideas/{slug}/research", m.handler.APIAddResearch)
}

// Watches covers every user's ideas.md, plus the owner's file when no-auth
// mode keeps it at IDEAS_PATH.
func (m *Module) Watches() []module.WatchSpec {
	specs := []module.WatchSpec{{UserFile: "ideas.md", Reload: func(uid int64) (bool, error) {
		return m.ideas(uid).ResyncIfChanged()
	}}}
	for _, o := range m.deps.Services.Overrides() {
		svc := m.ideas(o.UserID)
		specs = append(specs, module.WatchSpec{Path: o.Ideas, Reload: func(int64) (bool, error) { return svc.ResyncIfChanged() }})
	}
	return specs
}

func (m *Module) Search(ctx context.Context, userID int64, q string) []module.SearchResult {
	return search.Ideas(m.ideas).Search(ctx, userID, q)
}

// Widgets lists the first untriaged ideas, shown once any idea exists.
func (m *Module) Widgets(_ context.Context, userID int64, _ time.Time) []module.WidgetData {
	all, err := m.ideas(userID).List()
	if err != nil {
		slog.Error("ideas widget", "error", err)
		return nil
	}
	if len(all) == 0 {
		return nil
	}
	data := module.WidgetData{Title: "Ideas", CountLabel: "untriaged", Note: fmt.Sprintf("%d total", len(all)), Link: "/ideas"}
	for _, idea := range all {
		if idea.Status != "untriaged" {
			continue
		}
		data.Count++
		if len(data.Items) < untriagedShown {
			data.Items = append(data.Items, module.WidgetItem{Label: idea.Title, URL: "/ideas/" + idea.Slug})
		}
	}
	return []module.WidgetData{data}
}
