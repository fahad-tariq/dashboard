package test

import (
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
)

// TestRouteGolden pins every method and path the router exposes in each auth
// mode. Any diff to testdata/routes_*.golden must be explained in the plan's
// working notes.
func TestRouteGolden(t *testing.T) {
	tests := map[string]struct {
		passwordHash string
	}{
		"auth":   {passwordHash: "$2a$10$abcdefghijklmnopqrstuuJ5bW3pH0bQ0b9b7xg2y2Jm8m2pXq6m6"},
		"noauth": {passwordHash: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			paths := tempPaths(t)
			paths["DASHBOARD_API_TOKEN"] = strings.Repeat("x", 32)
			paths["DASHBOARD_PASSWORD_HASH"] = tc.passwordHash
			setEnvForConfig(t, paths)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("config.Load: %v", err)
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

			var routes []string
			walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
				routes = append(routes, fmt.Sprintf("%-7s %s", method, route))
				return nil
			}
			if err := chi.Walk(router, walk); err != nil {
				t.Fatalf("chi.Walk: %v", err)
			}
			slices.Sort(routes)
			assertGolden(t, "routes_"+name+".golden", []byte(strings.Join(routes, "\n")+"\n"))
		})
	}
}

func closeDB(t *testing.T, database *sql.DB) {
	t.Helper()
	if err := database.Close(); err != nil {
		t.Errorf("closing db: %v", err)
	}
}
