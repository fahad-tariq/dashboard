package test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
func renderRouter(t *testing.T) (http.Handler, *config.Config) {
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
		"- [ ] Renew passport !high [added: 2026-09-01] [planned: " + today + "]\n" +
		"- [ ] Plan weekend hike [added: 2026-09-20] [tags: outdoors]\n" +
		"  - [x] Pick a trail\n" +
		"  - [ ] Check the weather\n" +
		"- [ ] Read 12 books [goal: 3/12 books] [added: 2026-01-01]\n"
	if err := os.WriteFile(paths["PERSONAL_PATH"], []byte(personal), 0o600); err != nil {
		t.Fatal(err)
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

func TestLayoutInjectsContrastCheckedAccent(t *testing.T) {
	h, cfg := renderRouter(t)
	body := renderPage(t, h, "/")
	acc, err := seasonal.AccentFor(time.Now().In(cfg.Location), loadThemeTokens(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`[data-theme="light"] { --accent: ` + acc.Light.Hex() + `; }`,
		`[data-theme="dark"] { --accent: ` + acc.Dark.Hex() + `; }`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing %q", want)
		}
	}
}
