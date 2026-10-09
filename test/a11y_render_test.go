package test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/seasonal"
)

// renderRouter builds the real router in local no-auth mode with a personal
// list holding one task planned for today, one unplanned task with
// sub-steps, and one goal.
// IDs of the items renderRouter seeds.
const (
	renewPassport  = "r3nwpsp5"
	planHike       = "hk3w33k5"
	readBooks      = "r3dbkk12"
	pickupRoster   = "r5t3rpk2"
	weatherStation = "w3th3rst"
	learnSail      = "s41l1ng5"
	cleanGutters   = "gtt3rs55"
	paintFence     = "f3nc3p41"
)

func renderRouter(t *testing.T) (http.Handler, *config.Config) {
	t.Helper()
	return renderRouterWith(t, nil)
}

// renderRouterWith is renderRouter with a hook that can change the seed files
// (keyed by their path variable) before the router loads them.
func renderRouterWith(t *testing.T, edit func(seeds map[string]string, today string)) (http.Handler, *config.Config) {
	t.Helper()
	paths := tempPaths(t)
	paths["DASHBOARD_PASSWORD_HASH"] = ""
	paths["DASHBOARD_AUTH"] = "disabled"
	paths["ADDR"] = "127.0.0.1:0"
	setEnvForConfig(t, paths)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	today := time.Now().In(cfg.Location).Format("2006-01-02")
	personal := "# Personal\n\n" +
		"- [ ] Renew passport !high [added: 2026-09-01] [planned: " + today + "] [id: " + renewPassport + "]\n" +
		"- [ ] Plan weekend hike [added: 2026-09-20] [tags: outdoors] [id: " + planHike + "]\n" +
		"  - [x] Pick a trail\n" +
		"  - [ ] Check the weather\n" +
		"- [ ] Read 12 books [goal: 3/12 books] [added: 2026-01-01] [id: " + readBooks + "]\n"
	seeds := map[string]string{
		"PERSONAL_PATH": personal,
		"FAMILY_PATH": "# Family\n\n" +
			"- [ ] Organise school pickup roster !medium [added: 2026-09-15] [tags: kids] [planned: " + today + "] [id: " + pickupRoster + "]\n",
		"IDEAS_PATH": "# Ideas\n\n" +
			"- [ ] Home weather station [status: untriaged] [tags: electronics] [added: 2026-09-10] [id: " + weatherStation + "]\n" +
			"  Raspberry Pi with a BME280 sensor on the balcony.\n\n" +
			"- [ ] Learn to sail [status: parked] [tags: outdoors] [added: 2026-08-01] [id: " + learnSail + "]\n",
		"MAINTENANCE_PATH": "# Maintenance\n\n" +
			"- [ ] Clean gutters [cadence: 6m] [tags: exterior] [added: 2026-01-10] [id: " + cleanGutters + "]\n" +
			"  - [x] 2026-03-01 - front and back\n",
		"HOUSE_PROJECTS_PATH": "# House\n\n" +
			"- [ ] Paint the back fence [added: 2026-09-01] [tags: exterior] [budget: 400] [status: todo] [id: " + paintFence + "]\n",
	}
	if edit != nil {
		edit(seeds, today)
	}
	for key, content := range seeds {
		if err := os.WriteFile(paths[key], []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { closeDB(t, database) })
	router, err := app.NewRouter(t.Context(), cfg, database, "test")
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return router, cfg
}

func renderPage(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rr.Code)
	}
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestLayoutInjectsContrastCheckedSeasonalColour(t *testing.T) {
	h, cfg := renderRouter(t)
	body := renderPage(t, h, "/")
	c, err := seasonal.ColourFor(time.Now().In(cfg.Location), loadThemeTokens(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`[data-theme="light"] { --seasonal: ` + c.Light.Hex() + `; }`,
		`[data-theme="dark"] { --seasonal: ` + c.Dark.Hex() + `; }`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing %q", want)
		}
	}
}

var mainPages = []string{"/", "/todos", "/family", "/goals", "/ideas", "/ideas/" + weatherStation, "/house", "/digest", "/plan/calendar", "/plan/calendar?view=month"}

func TestLayoutLandmarksAndLiveRegions(t *testing.T) {
	h, _ := renderRouter(t)
	body := renderPage(t, h, "/todos")
	for _, want := range []string{
		`<a class="skip-link" href="#main">Skip to content</a>`,
		`<main id="main" class="container" tabindex="-1">`,
		`<div id="announcer" class="sr-only" role="status" aria-live="polite"></div>`,
		`<div id="toast" class="toast" hidden>`,
		`<meta name="color-scheme" content="light dark">`,
		`prefers-color-scheme: dark`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing %q", want)
		}
	}
	if skip, nav := strings.Index(body, `class="skip-link"`), strings.Index(body, `<nav`); skip > nav {
		t.Error("skip link must come before the nav")
	}
}

func TestHomepageHasH1(t *testing.T) {
	h, _ := renderRouter(t)
	if body := renderPage(t, h, "/"); !strings.Contains(body, "<h1") {
		t.Error("homepage has no <h1>")
	}
}

func TestFlashRenderedOnceWithRole(t *testing.T) {
	h, _ := renderRouter(t)
	body := renderPage(t, h, "/?msg=plan-set")
	if n := strings.Count(body, `class="flash-msg`); n != 1 {
		t.Errorf("flash rendered %d times, want 1", n)
	}
	if !strings.Contains(body, `class="flash-msg" data-error="false" role="status"`) {
		t.Error(`success flash should be role="status"`)
	}
}

func TestPagesHaveNoRoleButtonAndUniqueIDs(t *testing.T) {
	h, _ := renderRouter(t)
	idRe := regexp.MustCompile(`\sid="([^"]+)"`)
	for _, path := range mainPages {
		body := renderPage(t, h, path)
		if strings.Contains(body, `role="button"`) {
			t.Errorf("%s: uses role=\"button\"; use a <button>", path)
		}
		if regexp.MustCompile(`class="htmx-indicator[^"]*"[^>]*aria-live`).MatchString(body) {
			t.Errorf("%s: the refresh indicator is a live region; #announcer is the only one", path)
		}
		seen := map[string]bool{}
		for _, m := range idRe.FindAllStringSubmatch(body, -1) {
			if seen[m[1]] {
				t.Errorf("%s: duplicate id %q", path, m[1])
			}
			seen[m[1]] = true
		}
	}
}

func TestPlanRowMarkup(t *testing.T) {
	h, _ := renderRouter(t)
	body := renderPage(t, h, "/")
	for _, want := range []string{
		`id="plan-todos-r3nwpsp5"`,
		`<button type="button" class="plan-item-toggle" id="plan-todos-r3nwpsp5-toggle" data-row-focus data-keep-attr="aria-expanded" aria-expanded="false" aria-controls="plan-todos-r3nwpsp5-detail">`,
		`class="plan-item-detail" id="plan-todos-r3nwpsp5-detail"`,
		`id="plan-todos-r3nwpsp5-up" aria-label="Move Renew passport up"`,
		`id="plan-todos-r3nwpsp5-down" aria-label="Move Renew passport down"`,
		`id="plan-todos-r3nwpsp5-done"`,
		`id="plan-todos-r3nwpsp5-drop"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
}

func TestTrackerRowMarkup(t *testing.T) {
	h, _ := renderRouter(t)
	body := renderPage(t, h, "/todos")
	for _, want := range []string{
		`id="item-hk3w33k5"`,
		`id="item-hk3w33k5-toggle" data-row-focus data-keep-attr="aria-expanded" aria-expanded="false" aria-controls="item-hk3w33k5-detail" aria-label="Plan weekend hike"`,
		`class="tracker-item-detail" id="item-hk3w33k5-detail"`,
		`id="item-hk3w33k5-substep-0-toggle" aria-label="Mark step not done: Pick a trail"`,
		`id="item-hk3w33k5-substep-1-toggle" aria-label="Mark step done: Check the weather"`,
		`id="item-hk3w33k5-substep-1-promote" aria-label="Promote step to task: Check the weather"`,
		`id="item-hk3w33k5-substep-1-remove" aria-label="Remove step: Check the weather"`,
		`aria-label="Complete Plan weekend hike"`,
		`<span class="badge badge-tag">`,
		`<aside class="list-rail" aria-label="Todos overview and filters">`,
		`<span class="filter-count">`,
		`aria-label="Do today: Plan weekend hike" title="Do today">today</button>`,
		`<button type="submit" class="tick item-complete-btn" aria-label="Complete Plan weekend hike"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/todos missing %q", want)
		}
	}
}

// TestFormFieldsHaveLabels requires every visible form field to have an
// accessible name: aria-label, aria-labelledby or a <label for>.
func TestFormFieldsHaveLabels(t *testing.T) {
	h, _ := renderRouter(t)
	fieldRe := regexp.MustCompile(`<(input|textarea|select)\b[^>]*>`)
	typeRe := regexp.MustCompile(`\stype="(hidden|submit|button)"`)
	idRe := regexp.MustCompile(`\sid="([^"]+)"`)
	for _, path := range mainPages {
		body := renderPage(t, h, path)
		for _, field := range fieldRe.FindAllString(body, -1) {
			if typeRe.MatchString(field) || strings.Contains(field, "aria-label") {
				continue
			}
			if m := idRe.FindStringSubmatch(field); m != nil && strings.Contains(body, `for="`+m[1]+`"`) {
				continue
			}
			t.Errorf("%s: unlabelled field %s", path, field)
		}
	}
}

// A page whose nav link sits behind "more" marks the "more" button, so the
// current section stays visible while the menu is closed.
func TestMoreButtonMarksCurrentSection(t *testing.T) {
	h, _ := renderRouter(t)
	tests := map[string]struct {
		path       string
		wantMarked bool
	}{
		"digest is behind more":    {path: "/digest", wantMarked: true},
		"calendar is behind more":  {path: "/plan/calendar", wantMarked: true},
		"todos is in the main bar": {path: "/todos", wantMarked: false},
	}
	marked := `<button type="button" class="nav-more-btn nav-active" id="nav-more-btn"`
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			body := renderPage(t, h, tc.path)
			if got := strings.Contains(body, marked); got != tc.wantMarked {
				t.Errorf("more button marked = %v, want %v", got, tc.wantMarked)
			}
		})
	}
}
