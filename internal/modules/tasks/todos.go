package tasks

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/tracker"
)

// TodosID is the todos module's ID.
const TodosID = "todos"

// Todos is the user's own task list and goals, in USER_DATA_DIR/{id}/personal.md.
type Todos struct {
	deps    module.Deps
	handler *tracker.Handler
}

// NewTodos builds the todos module.
func NewTodos(deps module.Deps) module.Module {
	m := &Todos{deps: deps}
	m.handler = tracker.NewHandlerWithResolver(m.lists, deps.Render.Lookup(templates), TodosID, deps.Location)
	m.handler.SetCommentaryStore(deps.Commentary)
	return m
}

func (m *Todos) personal(userID int64) *tracker.Service {
	return m.deps.Services.ForUser(userID).Personal
}

// lists returns the request user's list and family, where "move" sends items.
func (m *Todos) lists(r *http.Request) (*tracker.Service, *tracker.Service) {
	return m.personal(auth.UserID(r.Context())), m.deps.Services.Family()
}

func (m *Todos) Manifest() module.Manifest {
	return module.Manifest{
		ID:    TodosID,
		Title: "Todos",
		Nav: []module.NavItem{
			{Label: "todos", Path: "/todos", Order: 10, Group: module.Primary, Shortcut: "t"},
			{Label: "goals", Path: "/goals", Order: 20, Group: module.Primary, Shortcut: "o"},
		},
		Prefixes:  []string{"/todos", "/goals", "/personal"},
		Templates: templates,
	}
}

func (m *Todos) Routes(r chi.Router) {
	r.Get("/todos", m.handler.TrackerPage)
	r.Get("/goals", m.handler.GoalsPage)
	r.Get("/personal", http.RedirectHandler("/todos", http.StatusMovedPermanently).ServeHTTP)
	m.handler.Mount(r, "/todos")
	r.Post("/todos/add-goal", m.handler.AddGoal)
}

func (m *Todos) APIRoutes(r chi.Router) {
	r.Get("/todos", tracker.APIListTodos(m.lists))
	r.Post("/todos", tracker.APIAddTodo(m.lists))
	r.Get("/todos/"+itemid.Route, tracker.APIGetTodo(m.lists))
	r.Put("/todos/"+itemid.Route, tracker.APIUpdateTodo(m.lists))
	r.Post("/todos/"+itemid.Route+"/complete", tracker.APICompleteTodo(m.lists))
	r.Post("/todos/"+itemid.Route+"/uncomplete", tracker.APIUncompleteTodo(m.lists))
	r.Delete("/todos/"+itemid.Route, tracker.APIDeleteTodo(m.lists))
	r.Put("/todos/"+itemid.Route+"/priority", tracker.APIUpdatePriority(m.lists))
	r.Put("/todos/"+itemid.Route+"/tags", tracker.APIUpdateTags(m.lists))
	r.Post("/todos/"+itemid.Route+"/substeps", tracker.APIAddSubStep(m.lists))
	r.Put("/todos/"+itemid.Route+"/substeps/{index}", tracker.APIToggleSubStep(m.lists))
	r.Delete("/todos/"+itemid.Route+"/substeps/{index}", tracker.APIRemoveSubStep(m.lists))
}

// Watches covers every user's personal.md, plus the owner's file when
// no-auth mode keeps it at PERSONAL_PATH.
func (m *Todos) Watches() []module.WatchSpec {
	specs := []module.WatchSpec{{UserFile: "personal.md", Reload: func(uid int64) (bool, error) {
		return m.personal(uid).ResyncIfChanged()
	}}}
	for _, o := range m.deps.Services.Overrides() {
		specs = append(specs, module.WatchSpec{Path: o.Personal, Reload: resync(m.personal(o.UserID))})
	}
	return specs
}

func (m *Todos) Search(ctx context.Context, userID int64, q string) []module.SearchResult {
	return search.Tracker(TodosID, "/todos#item-", m.personal).Search(ctx, userID, q)
}

// Widgets shows tasks due soon from the user's list and family, then open
// tasks and active goals.
func (m *Todos) Widgets(_ context.Context, userID int64, now time.Time) []module.WidgetData {
	svc := m.personal(userID)
	var out []module.WidgetData
	if w, ok := dueWidget([]dueList{{list: TodosID, svc: svc}, {list: FamilyID, context: "Family", svc: m.deps.Services.Family()}}, now); ok {
		out = append(out, w)
	}
	if w, ok := taskWidget("", "Todos", "/todos", svc, now.Format("2006-01-02")); ok {
		out = append(out, w)
	}
	if w, ok := goalsWidget(svc, now); ok {
		out = append(out, w)
	}
	return out
}

func goalsWidget(svc *tracker.Service, now time.Time) (module.WidgetData, bool) {
	items, err := svc.List()
	if err != nil {
		return module.WidgetData{}, false
	}
	data := module.WidgetData{ID: "goals", Title: "Goals", CountLabel: "active", Link: "/goals"}
	for _, it := range items {
		if it.Type != tracker.GoalType || it.Done {
			continue
		}
		data.Count++
		row := module.WidgetItem{Label: it.Title, URL: "/goals#item-" + it.ID}
		if it.Target > 0 {
			label := formatNum(it.Current) + "/" + formatNum(it.Target)
			if it.Unit != "" {
				label += " " + it.Unit
			}
			row.Progress = &module.WidgetProgress{
				Percent: max(0, min(int(it.Current/it.Target*100), 100)),
				Label:   label,
				Class:   insights.ProgressColour(it.Current, it.Target, it.Added, it.Deadline, now),
			}
		}
		data.Items = append(data.Items, row)
	}
	return data, data.Count > 0
}
