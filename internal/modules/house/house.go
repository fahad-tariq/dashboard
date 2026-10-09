// Package house is the house module: recurring maintenance in
// MAINTENANCE_PATH and projects in HOUSE_PROJECTS_PATH, on one page. The two
// stay in separate files because each service rewrites its whole file.
package house

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	houselib "github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/tracker"
)

// ID is the module's ID and template directory.
const ID = "house"

// Module is the house module.
type Module struct {
	deps    module.Deps
	handler *houselib.Handler
}

// New builds the house module.
func New(deps module.Deps) module.Module {
	m := &Module{deps: deps}
	m.handler = houselib.NewHandler(deps.Services.Maintenance(), deps.Services.HouseProjects(), deps.Render.Lookup(ID), deps.Location)
	m.handler.SetCommentaryStore(deps.Commentary)
	return m
}

func (m *Module) Manifest() module.Manifest {
	return module.Manifest{
		ID:       ID,
		Title:    "House",
		Nav:      []module.NavItem{{Label: "house", Path: "/house", Order: 40, Group: module.Primary, Shortcut: "u"}},
		Prefixes: []string{"/house"},
	}
}

func (m *Module) Routes(r chi.Router) {
	h := m.handler
	r.Get("/house", h.HousePage)
	r.Post("/house/maintenance/add", h.AddMaintenance)
	r.Post("/house/maintenance/"+itemid.Route+"/log", h.LogDone)
	r.Post("/house/maintenance/"+itemid.Route+"/edit", h.EditMaintenance)
	r.Post("/house/maintenance/"+itemid.Route+"/delete", h.DeleteMaintenance)
	r.Post("/house/maintenance/"+itemid.Route+"/restore", h.RestoreMaintenance)
	r.Post("/house/maintenance/"+itemid.Route+"/purge", h.PurgeMaintenance)
	r.Post("/house/projects/add", h.AddProject)
	r.Post("/house/projects/"+itemid.Route+"/edit", h.EditProject)
	r.Post("/house/projects/"+itemid.Route+"/complete", h.CompleteProject)
	r.Post("/house/projects/"+itemid.Route+"/uncomplete", h.UncompleteProject)
	r.Post("/house/projects/"+itemid.Route+"/status", h.UpdateStatus)
	r.Post("/house/projects/"+itemid.Route+"/delete", h.DeleteProject)
	r.Post("/house/projects/"+itemid.Route+"/restore", h.RestoreProject)
	r.Post("/house/projects/"+itemid.Route+"/purge", h.PurgeProject)
}

func (m *Module) Watches() []module.WatchSpec {
	projects, maintenance := m.deps.Services.HouseProjects(), m.deps.Services.Maintenance()
	return []module.WatchSpec{
		{Path: m.deps.Config.HouseProjectsPath, Reload: func(int64) (bool, error) { return projects.ResyncIfChanged() }},
		{Path: m.deps.Config.MaintenancePath, Reload: func(int64) (bool, error) { return maintenance.ResyncIfChanged() }},
	}
}

// Search finds projects, then maintenance items.
func (m *Module) Search(ctx context.Context, userID int64, q string) []module.SearchResult {
	projects := func(int64) *tracker.Service { return m.deps.Services.HouseProjects() }
	return append(
		search.Tracker(ID, "/house#item-", projects).Search(ctx, userID, q),
		search.Maintenance(m.deps.Services.Maintenance()).Search(ctx, userID, q)...,
	)
}

// Widgets lists overdue maintenance.
func (m *Module) Widgets(_ context.Context, _ int64, now time.Time) []module.WidgetData {
	overdue := m.deps.Services.Maintenance().ListOverdue(now)
	if len(overdue) == 0 {
		return nil
	}
	data := module.WidgetData{Title: "House Maintenance", Count: len(overdue), CountLabel: "overdue", Link: "/house"}
	for _, it := range overdue {
		data.Items = append(data.Items, module.WidgetItem{Label: it.Title, URL: "/house#maint-" + it.ID, Severity: module.SeverityDanger})
	}
	return []module.WidgetData{data}
}
