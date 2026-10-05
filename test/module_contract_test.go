package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/module/moduletest"
	"github.com/fahad/dashboard/internal/sse"
)

type contractEnv struct {
	h      http.Handler
	cookie *http.Cookie
	file   string // the fixture's watched file
	events chan string
	token  string
}

// newContractEnv builds the real router in auth mode with the fixture module
// registered and a broker the test listens to.
func newContractEnv(t *testing.T) *contractEnv {
	t.Helper()
	paths := tempPaths(t)
	paths["DASHBOARD_PASSWORD_HASH"] = ""
	paths["DASHBOARD_SECURE_COOKIES"] = "false"
	token := strings.Repeat("t", 32)
	paths["DASHBOARD_API_TOKEN"] = token
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
	broker := sse.NewBroker()
	h, err := app.NewRouterWith(t.Context(), cfg, database, "test", app.Options{
		Modules: []module.Factory{moduletest.New},
		Broker:  broker,
	})
	if err != nil {
		t.Fatalf("NewRouterWith: %v", err)
	}
	events := make(chan string, 32)
	broker.Subscribe(events)
	t.Cleanup(func() { broker.Unsubscribe(events) })
	return &contractEnv{
		h:      h,
		cookie: login(t, h, "owner@test.com", "owner-password"),
		file:   filepath.Join(filepath.Dir(cfg.FamilyPath), "moduletest.md"),
		events: events,
		token:  token,
	}
}

func (e *contractEnv) get(t *testing.T, path string) string {
	t.Helper()
	rr := getAs(t, e.h, e.cookie, path)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rr.Code)
	}
	return rr.Body.String()
}

// waitEvent waits for an SSE message naming event, ignoring others.
func (e *contractEnv) waitEvent(t *testing.T, event string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case msg := <-e.events:
			if strings.HasPrefix(msg, "event: "+event+"\n") {
				return
			}
		case <-deadline:
			t.Fatalf("no %q event within %s", event, within)
		}
	}
}

// The fixture module's capabilities each show up in the right place.
func TestModuleContract(t *testing.T) {
	env := newContractEnv(t)

	t.Run("nav link behind more, with shortcut and help row", func(t *testing.T) {
		body := env.get(t, "/")
		more := regexp.MustCompile(`(?s)<div class="nav-more-menu" id="nav-more-menu" hidden>(.*?)</div>`).FindStringSubmatch(body)
		if more == nil || !strings.Contains(more[1], `<a href="/moduletest" data-shortcut="x">fixture</a>`) {
			t.Errorf("fixture link not in the more menu")
		}
		if !strings.Contains(body, `<span>Go to fixture</span> <kbd>g x</kbd>`) {
			t.Error("shortcut help does not list g x")
		}
	})

	t.Run("own page marks its nav link current", func(t *testing.T) {
		body := env.get(t, "/moduletest")
		if !strings.Contains(body, `<a href="/moduletest" data-shortcut="x" class="nav-active" aria-current="page">fixture</a>`) {
			t.Error("fixture link lacks aria-current on its own page")
		}
		if !strings.Contains(body, `<a href="/" data-shortcut="h">home</a>`) {
			t.Error("home link should not be current on /moduletest")
		}
	})

	t.Run("empty state through the shared partial", func(t *testing.T) {
		body := env.get(t, "/moduletest")
		for _, want := range []string{`<header class="page-header">`, `<p class="empty">No fixture items yet.</p>`, `<details class="quick-add">`} {
			if !strings.Contains(body, want) {
				t.Errorf("page lacks %s", want)
			}
		}
	})

	t.Run("unauthenticated request redirects to login", func(t *testing.T) {
		rr := getAs(t, env.h, nil, "/moduletest")
		if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login?next=%2Fmoduletest" {
			t.Errorf("got %d to %q, want 303 to login", rr.Code, rr.Header().Get("Location"))
		}
	})

	t.Run("add publishes changed:moduletest and shows the item", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/moduletest/add", strings.NewReader(url.Values{"title": {"Polish the brass"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(env.cookie)
		rr := httptest.NewRecorder()
		env.h.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("POST add: status %d", rr.Code)
		}
		env.waitEvent(t, "changed:moduletest", 3*time.Second)
		body := env.get(t, "/moduletest?msg=added")
		if !strings.Contains(body, `<div class="item-row" id="moduletest-0">`) || !strings.Contains(body, "Polish the brass") {
			t.Error("added item not rendered through item-row")
		}
		if !strings.Contains(body, `role="status">Added.</div>`) {
			t.Error("manifest flash message not rendered")
		}
	})

	t.Run("search returns the module's results", func(t *testing.T) {
		body := env.get(t, "/search?q=brass")
		if !strings.Contains(body, "Polish the brass") || !strings.Contains(body, ">moduletest<") {
			t.Errorf("search lacks the fixture result:\n%s", body)
		}
	})

	t.Run("home widget, with and without the plan section", func(t *testing.T) {
		// The owner has no tasks yet, so the homepage shows its onboarding
		// message; widgets render there too.
		body := env.get(t, "/")
		if !strings.Contains(body, `<section class="homepage-card" aria-labelledby="widget-moduletest-title">`) {
			t.Fatal("no fixture widget on the empty homepage")
		}
		req := httptest.NewRequest("POST", "/todos/add", strings.NewReader(url.Values{"title": {"Water plants"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(env.cookie)
		env.h.ServeHTTP(httptest.NewRecorder(), req)
		body = env.get(t, "/")
		plan := strings.Index(body, `id="plan-section"`)
		widget := strings.Index(body, `<section class="homepage-card" aria-labelledby="widget-moduletest-title">`)
		if plan < 0 || widget < plan {
			t.Errorf("widget at %d, plan section at %d: want the widget below the plan", widget, plan)
		}
		if !strings.Contains(body, `<h2 class="homepage-card-title" id="widget-moduletest-title"><a href="/moduletest">Fixture items</a></h2>`) {
			t.Error("widget heading missing or not linked")
		}
	})

	t.Run("API routes sit behind the bearer token, under /api/v1/<id>", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/moduletest", nil)
		rr := httptest.NewRecorder()
		env.h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("without token: status %d, want 401", rr.Code)
		}
		req.Header.Set("Authorization", "Bearer "+env.token)
		rr = httptest.NewRecorder()
		env.h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"count":1`) {
			t.Errorf("with token: %d %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("external edit to the watched file emits changed:moduletest", func(t *testing.T) {
		time.Sleep(700 * time.Millisecond) // let earlier debounced events drain
		for len(env.events) > 0 {
			<-env.events
		}
		if err := os.WriteFile(env.file, []byte("- Polish the brass\n- Edited by hand\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		env.waitEvent(t, "changed:moduletest", 3*time.Second)
	})

	t.Run("error banner through the shared partial", func(t *testing.T) {
		if err := os.Remove(env.file); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(env.file, 0o755); err != nil { // reading a directory fails
			t.Fatal(err)
		}
		body := env.get(t, "/moduletest")
		if !strings.Contains(body, `<div class="flash-msg flash-msg-error" role="alert">Could not read the fixture list.</div>`) {
			t.Error("error banner missing")
		}
		if strings.Contains(body, `<p class="empty">`) {
			t.Error("empty state shown alongside the error")
		}
	})
}
