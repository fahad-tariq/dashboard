package test

import (
	"testing"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
)

const testBcryptHash = "$2a$10$abcdefghijklmnopqrstuuJ5bW3pH0bQ0b9b7xg2y2Jm8m2pXq6m6"

// Auth must never be off by accident: a lost data volume (no users, no hash)
// has to stop the server rather than serve an open dashboard.
func TestAuthModeAtStartup(t *testing.T) {
	tests := map[string]struct {
		hash     string
		withUser bool
		authEnv  string
		addr     string
		wantErr  bool
		wantAuth bool
	}{
		"no hash, no users, no switch":    {addr: ":8080", wantErr: true},
		"hash configured":                 {hash: testBcryptHash, addr: ":8080", wantAuth: true},
		"existing user, no hash":          {withUser: true, addr: ":8080", wantAuth: true},
		"disabled on all interfaces":      {authEnv: "disabled", addr: ":8080", wantErr: true},
		"disabled on 0.0.0.0":             {authEnv: "disabled", addr: "0.0.0.0:8080", wantErr: true},
		"disabled on LAN address":         {authEnv: "disabled", addr: "192.168.1.5:8080", wantErr: true},
		"disabled on 127.0.0.1":           {authEnv: "disabled", addr: "127.0.0.1:8080"},
		"disabled on ::1":                 {authEnv: "disabled", addr: "[::1]:8080"},
		"disabled on localhost":           {authEnv: "disabled", addr: "localhost:8080"},
		"disabled overrides a stray hash": {authEnv: "disabled", hash: testBcryptHash, addr: "127.0.0.1:8080"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			paths := tempPaths(t)
			paths["ADDR"] = tc.addr
			paths["DASHBOARD_PASSWORD_HASH"] = tc.hash
			paths["DASHBOARD_AUTH"] = tc.authEnv
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
			if tc.withUser {
				if _, err := auth.CreateUser(database, "owner@example.com", "", "correct-horse"); err != nil {
					t.Fatal(err)
				}
			}

			_, err = app.NewRouter(t.Context(), cfg, database, "test")
			if tc.wantErr {
				if err == nil {
					t.Fatal("NewRouter succeeded; want a startup error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRouter: %v", err)
			}
			if got := cfg.AuthEnabled(); got != tc.wantAuth {
				t.Errorf("AuthEnabled() = %v, want %v", got, tc.wantAuth)
			}
		})
	}
}

func TestConfigRejectsUnknownAuthMode(t *testing.T) {
	paths := tempPaths(t)
	paths["DASHBOARD_AUTH"] = "off"
	setEnvForConfig(t, paths)
	if _, err := config.Load(); err == nil {
		t.Fatal(`config.Load accepted DASHBOARD_AUTH="off"; want an error`)
	}
}
