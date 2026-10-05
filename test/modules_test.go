package test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/module"
)

// The built-in features are modules: an external edit to a file sends only
// its module's event, and nothing sends the old unscoped "file-changed".
func TestModuleFilesSendTheirOwnEvent(t *testing.T) {
	env := newAppEnv(t)
	userDir := filepath.Join(env.cfg.UserDataDir, "1")
	tests := map[string]struct {
		path    string
		content string
		want    string
	}{
		"personal tasks":  {filepath.Join(userDir, "personal.md"), "# Personal\n\n- [ ] Edited by hand\n", "changed:todos"},
		"ideas":           {filepath.Join(userDir, "ideas.md"), "# Ideas\n\n- [ ] Edited by hand [status: untriaged]\n", "changed:ideas"},
		"family tasks":    {env.cfg.FamilyPath, "# Family\n\n- [ ] Edited by hand\n", "changed:family"},
		"house projects":  {env.cfg.HouseProjectsPath, "# House\n\n- [ ] Edited by hand\n", "changed:house"},
		"maintenance log": {env.cfg.MaintenancePath, "# Maintenance\n\n- [ ] Gutters [cadence: 3m]\n", "changed:house"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env.drainEvents()
			if err := os.WriteFile(tc.path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := env.eventsWithin(1500 * time.Millisecond)
			if !slices.Equal(got, []string{tc.want}) {
				t.Errorf("events = %v, want [%s]", got, tc.want)
			}
		})
	}
}

// A write through a module's own routes publishes that module's event once.
func TestModuleWritesSendTheirOwnEvent(t *testing.T) {
	env := newAppEnv(t)
	tests := map[string]struct {
		path string
		form url.Values
		want string
	}{
		"todo":        {"/todos/add", url.Values{"title": {"Water plants"}}, "changed:todos"},
		"family task": {"/family/add", url.Values{"title": {"Book holiday"}}, "changed:family"},
		"idea":        {"/ideas/add", url.Values{"title": {"Solar oven"}}, "changed:ideas"},
		"maintenance": {"/house/maintenance/add", url.Values{"title": {"Gutters"}, "cadence": {"3m"}}, "changed:house"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env.drainEvents()
			if rr := env.post(t, tc.path, tc.form); rr.Code != http.StatusSeeOther {
				t.Fatalf("POST %s: status %d", tc.path, rr.Code)
			}
			got := env.eventsWithin(1500 * time.Millisecond)
			if !slices.Equal(got, []string{tc.want}) {
				t.Errorf("events = %v, want [%s]", got, tc.want)
			}
		})
	}
}

var sseTrigger = regexp.MustCompile(`hx-trigger="(sse:[^"]*)"`)

// Each page refreshes on its own module's event only; the homepage refreshes
// on every module that contributes to it.
func TestPagesRefreshOnTheirModulesEvents(t *testing.T) {
	env := newAppEnv(t)
	tests := map[string]struct {
		path string
		want []string
	}{
		"todos":    {"/todos", []string{"sse:changed:todos"}},
		"goals":    {"/goals", []string{"sse:changed:todos"}},
		"family":   {"/family", []string{"sse:changed:family"}},
		"ideas":    {"/ideas", []string{"sse:changed:ideas"}},
		"house":    {"/house", []string{"sse:changed:house"}},
		"homepage": {"/", []string{"sse:changed:todos", "sse:changed:family", "sse:changed:house", "sse:changed:ideas"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := sseTrigger.FindStringSubmatch(env.get(t, tc.path))
			if m == nil {
				t.Fatal("no SSE trigger on the page")
			}
			got := strings.Split(m[1], ", ")
			slices.Sort(got)
			want := slices.Sorted(slices.Values(tc.want))
			if !slices.Equal(got, want) {
				t.Errorf("hx-trigger = %q, want %v", m[1], want)
			}
		})
	}
}

// The homepage summary cards are module widgets. Links to tracker items use
// the row ids (item-{slug}); "#{slug}" never matched an element.
func TestHomepageWidgetsComeFromModules(t *testing.T) {
	env := newAppEnv(t)
	userDir := filepath.Join(env.cfg.UserDataDir, "1")
	files := map[string]string{
		filepath.Join(userDir, "personal.md"): "# Personal\n\n- [ ] Water plants !high\n- [ ] Run 100km [goal: 20/100 km]\n",
		filepath.Join(userDir, "ideas.md"):    "# Ideas\n\n- [ ] Solar oven [status: untriaged]\n",
		env.cfg.FamilyPath:                    "# Family\n\n- [ ] Book holiday\n",
		env.cfg.MaintenancePath:               "# Maintenance\n\n- [ ] Clean gutters [cadence: 3m]\n  - [x] 2020-01-01\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env.drainEvents() // the watcher reloads the services
	time.Sleep(time.Second)

	body := env.get(t, "/")
	tests := map[string]struct{ id, want string }{
		"todos":       {"widget-todos", `<a href="/todos#item-water-plants">Water plants</a>`},
		"goals":       {"widget-todos-goals", "Run 100km"},
		"family":      {"widget-family", `<a href="/family#item-book-holiday">Book holiday</a>`},
		"maintenance": {"widget-house", `<a href="/house#maint-clean-gutters">Clean gutters</a>`},
		"ideas":       {"widget-ideas", `<a href="/ideas/solar-oven">Solar oven</a>`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			section := regexp.MustCompile(`(?s)<section class="homepage-card" aria-labelledby="` + tc.id + `-title">(.*?)</section>`).FindStringSubmatch(body)
			if section == nil {
				t.Fatalf("no %s widget", tc.id)
			}
			if !strings.Contains(section[1], tc.want) {
				t.Errorf("%s widget lacks %s:\n%s", tc.id, tc.want, section[1])
			}
		})
	}

	t.Run("search links tracker results to their rows", func(t *testing.T) {
		if got := env.get(t, "/search?q=water"); !strings.Contains(got, `href="/todos#item-water-plants"`) {
			t.Errorf("search result does not link to the row:\n%s", got)
		}
	})
}

// apiModule registers an API route outside /<id>.
type apiModule struct{ fakeModule }

func (apiModule) APIRoutes(r chi.Router) {
	r.Get("/elsewhere", func(http.ResponseWriter, *http.Request) {})
}

// routeModule registers a page outside its declared prefixes.
type routeModule struct{ fakeModule }

func (routeModule) Routes(r chi.Router) {
	r.Get("/elsewhere", func(http.ResponseWriter, *http.Request) {})
}

// Module routes are checked against the module's prefixes at start-up, since
// chi would let a later route silently replace a core one.
func TestModuleRoutesMustStayUnderTheirPrefixes(t *testing.T) {
	tests := map[string]module.Factory{
		"API route outside /api/v1/<id>": func(module.Deps) module.Module {
			return apiModule{fakeModule{man: module.Manifest{ID: "stray", Prefixes: []string{"/stray"}}}}
		},
		"page outside the manifest prefixes": func(module.Deps) module.Module {
			return routeModule{fakeModule{man: module.Manifest{ID: "stray", Prefixes: []string{"/stray"}}}}
		},
	}
	for name, factory := range tests {
		t.Run(name, func(t *testing.T) {
			err := buildRouterErr(t, factory)
			if err == nil || !strings.Contains(err.Error(), "/elsewhere") {
				t.Errorf("err = %v, want a complaint about /elsewhere", err)
			}
		})
	}
}

// buildRouterErr builds the real router with one extra module and returns
// the start-up error.
func buildRouterErr(t *testing.T, extra module.Factory) error {
	t.Helper()
	paths := tempPaths(t)
	paths["DASHBOARD_API_TOKEN"] = strings.Repeat("t", 32)
	setEnvForConfig(t, paths)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, database) })
	if _, err := auth.CreateUser(database, "owner@test.com", "", "owner-password"); err != nil {
		t.Fatal(err)
	}
	_, err = app.NewRouterWith(t.Context(), cfg, database, "test", app.Options{Modules: []module.Factory{extra}})
	return err
}
