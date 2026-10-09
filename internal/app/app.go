// Package app builds the dashboard's HTTP router and starts its background
// work (file watcher, session cleanup, trash purge).
package app

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/account"
	"github.com/fahad/dashboard/internal/admin"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/commentary"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/home"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/modules/house"
	"github.com/fahad/dashboard/internal/modules/ideas"
	"github.com/fahad/dashboard/internal/modules/tasks"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/upload"
	"github.com/fahad/dashboard/internal/watcher"
)

const (
	trashRetentionDays = 7

	// ownerID is the user that no-auth mode serves and the API token acts as.
	ownerID int64 = 1

	// minAPITokenLength is the shortest DASHBOARD_API_TOKEN accepted; with a
	// shorter or missing token the API is not mounted at all.
	minAPITokenLength = 32

	// failedBearerLimit is how many bad bearer tokens one client IP may send
	// per minute before getting 429s. A valid token is never blocked.
	failedBearerLimit = 10

	// publishDebounce delays service change events like the old watcher
	// debounce, so the tab making a change finishes its request first.
	publishDebounce = 500 * time.Millisecond

	// sessionIdleTimeout logs out a session unused for a week, well inside
	// the absolute SESSION_LIFETIME.
	sessionIdleTimeout = 7 * 24 * time.Hour

	contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'"
)

// modules are the feature modules every build registers. Registration order
// sets the order of home widgets and search results; nav follows each item's
// Order.
var modules = []module.Factory{
	tasks.NewTodos,
	tasks.NewFamily,
	house.New,
	ideas.New,
}

// coreNav is the nav for the core's own pages.
var coreNav = []module.NavItem{
	{Label: "home", Path: "/", Order: 0, Group: module.Primary, Shortcut: "h"},
	{Label: "digest", Path: "/digest", Order: 60, Group: module.More, Shortcut: "d"},
	{Label: "calendar", Path: "/plan/calendar", Order: 70, Group: module.More, Shortcut: "c"},
}

// Options adds wiring that only tests need.
type Options struct {
	// Modules are registered after the built-in ones.
	Modules []module.Factory
	// Broker, if set, is used instead of a new one so a test can listen.
	Broker *sse.Broker
}

// NewRouter wires services, handlers and routes for cfg. Background goroutines
// it starts stop when shutdownCtx is cancelled; the file watcher runs for the
// life of the process.
func NewRouter(shutdownCtx context.Context, cfg *config.Config, database *sql.DB, version string) (*chi.Mux, error) {
	return NewRouterWith(shutdownCtx, cfg, database, version, Options{})
}

// NewRouterWith is NewRouter with extra modules or a given broker.
func NewRouterWith(shutdownCtx context.Context, cfg *config.Config, database *sql.DB, version string, opts Options) (*chi.Mux, error) {
	if err := prepareUsers(cfg, database); err != nil {
		return nil, err
	}

	broker := opts.Broker
	if broker == nil {
		broker = sse.NewBroker()
	}
	// End open event streams as soon as shutdown starts, so http.Server.Shutdown
	// is not left waiting on them.
	context.AfterFunc(shutdownCtx, broker.Close)

	// One publisher for services and modules, so a write sends one event
	// however it is reported.
	publish := broker.DebouncedChanged(publishDebounce)
	svcs, err := startServices(cfg, database, publish)
	if err != nil {
		return nil, err
	}
	commentaryStore := commentary.NewStore(database)
	reg, render, err := buildModules(cfg, svcs, commentaryStore, publish, slices.Concat(modules, opts.Modules))
	if err != nil {
		return nil, err
	}
	tmpls, err := loadTemplates(cfg, version, reg)
	if err != nil {
		return nil, err
	}
	render.SetPages(tmpls.modules)
	svcs.watch(cfg, broker, reg)
	go runHourly(shutdownCtx, func() { svcs.purgeExpired(database, commentaryStore) })

	sm := newSessionManager(shutdownCtx, cfg, database)
	h := svcs.handlers(cfg, database, sm, broker, commentaryStore, tmpls.core, tmpls.login, reg)

	root := chi.NewRouter()
	root.Use(securityHeaders)
	root.Use(middleware.Recoverer)
	root.Use(middleware.Compress(5))

	// Every browser-facing route sits behind cross-origin protection; the
	// bearer-token API is registered on root, outside it.
	r := root.With(http.NewCrossOriginProtection().Handler, fragmentResponses(func() http.Handler { return root }, broker.Revisions))
	r.Handle("/static/*", tmpls.assets.Handler())
	if err := mountBrowserRoutes(r, cfg, database, sm, h, reg); err != nil {
		return nil, err
	}
	if err := mountAPIRoutes(root, cfg, h, reg); err != nil {
		return nil, err
	}
	return root, nil
}

// buildModules constructs the modules and validates them against the core.
// The renderer is filled once templates are parsed, which needs the nav.
func buildModules(cfg *config.Config, svcs appServices, commentaryStore *commentary.Store, publish func(string), factories []module.Factory) (*module.Registry, *module.Renderer, error) {
	render := &module.Renderer{}
	deps := module.Deps{
		Location:   cfg.Location,
		Config:     cfg,
		Services:   svcs.registry,
		Render:     render,
		DataDir:    filepath.Dir(cfg.FamilyPath),
		Commentary: commentaryStore,
		Publish:    publish,
	}
	mods := make([]module.Module, 0, len(factories))
	for _, f := range factories {
		mods = append(mods, f(deps))
	}
	reg, err := module.NewRegistry(module.Core{Nav: coreNav}, mods...)
	if err != nil {
		return nil, nil, fmt.Errorf("registering modules: %w", err)
	}
	return reg, render, nil
}

// prepareUsers creates the users start-up needs and refuses unsafe auth
// configurations.
func prepareUsers(cfg *config.Config, database *sql.DB) error {
	// Legacy password migration: auto-create admin user if DASHBOARD_PASSWORD_HASH
	// is set and nobody can log in. The no-auth placeholder user does not count,
	// so a database first used locally cannot start in auth mode locked.
	count, err := auth.LoginUserCount(database)
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}
	if count == 0 && cfg.PasswordHash != "" {
		if _, err := auth.CreateUserWithHash(database, "admin@localhost", "", cfg.PasswordHash); err != nil {
			return fmt.Errorf("creating legacy admin user: %w", err)
		}
		slog.Info("auto-created admin@localhost from DASHBOARD_PASSWORD_HASH -- update your email with a new user")
		count = 1
	}
	cfg.HasUsers = count > 0
	if err := cfg.CheckAuthMode(); err != nil {
		return err
	}
	if cfg.AuthDisabled {
		// No-auth mode serves the owner, and the trash purge iterates users.
		return auth.EnsureUser(database, ownerID, "local@localhost")
	}
	return nil
}

// appServices are the long-lived services behind every handler.
type appServices struct {
	registry *services.Registry
}

// startServices builds the services, loads every known user's lists and
// makes each service publish its module's event after its own writes.
func startServices(cfg *config.Config, database *sql.DB, publish func(moduleID string)) (appServices, error) {
	registry := services.NewRegistry(database, cfg.UserDataDir, cfg.FamilyPath, cfg.HouseProjectsPath, cfg.MaintenancePath, cfg.Location)
	if cfg.AuthDisabled {
		// Local development keeps PERSONAL_PATH and IDEAS_PATH.
		registry.SetUserPaths(ownerID, cfg.PersonalPath, cfg.IdeasPath)
	}
	svcs := appServices{registry: registry}

	users, err := auth.AllUsers(database)
	if err != nil {
		return svcs, fmt.Errorf("loading users for directory provisioning: %w", err)
	}
	for _, u := range users {
		if err := registry.EnsureUserDirs(u.ID); err != nil {
			slog.Error("provisioning user dirs", "user_id", u.ID, "error", err)
		}
	}
	if err := registry.Family().Resync(); err != nil {
		slog.Warn("initial family sync", "error", err)
	}
	if err := registry.HouseProjects().Resync(); err != nil {
		slog.Warn("initial house projects sync", "error", err)
	}
	for _, u := range users {
		if err := registry.ForUser(u.ID).Personal.Resync(); err != nil {
			slog.Warn("initial personal sync", "user_id", u.ID, "error", err)
		}
	}

	registry.SetPublisher(publish)
	return svcs, nil
}

// watch starts the file watcher over every module's watched files.
func (s appServices) watch(cfg *config.Config, broker *sse.Broker, reg *module.Registry) {
	var specs []watcher.Spec
	for _, m := range reg.Modules() {
		w, ok := m.(module.Watcher)
		if !ok {
			continue
		}
		id := m.Manifest().ID
		for _, ws := range w.Watches() {
			reload := ws.Reload
			specs = append(specs, watcher.Spec{Path: ws.Path, UserFile: ws.UserFile, Event: "changed:" + id, Data: id, Reload: func(uid int64) bool {
				changed, err := reload(uid)
				if err != nil {
					slog.Error("module reload failed", "module", id, "user_id", uid, "error", err)
					return false
				}
				return changed
			}})
		}
	}
	if err := watcher.Watch(cfg.UserDataDir, specs, broker); err != nil {
		slog.Warn("file watcher failed to start", "error", err)
	}
}

// lists resolves the lists the core's planner, digest and tag summary read.
func (s appServices) lists(r *http.Request) home.Lists {
	u := s.registry.ForUser(auth.UserID(r.Context()))
	return home.Lists{
		Personal:      u.Personal,
		Family:        s.registry.Family(),
		HouseProjects: s.registry.HouseProjects(),
		Ideas:         u.Ideas,
	}
}

func (s appServices) handlers(cfg *config.Config, database *sql.DB, sm *scs.SessionManager, broker *sse.Broker, commentaryStore *commentary.Store, templates map[string]*template.Template, loginTmpl *template.Template, reg *module.Registry) *handlers {
	h := &handlers{
		home:       home.NewHandler(s.lists, templates, cfg.Location),
		search:     search.NewHandler(reg.Searchers()),
		upload:     upload.NewHandler(cfg.UploadsDir),
		account:    account.NewHandler(database, sm, templates),
		admin:      admin.NewHandler(database, s.registry, cfg.UserDataDir, templates),
		auth:       auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl, auth.WithTrustedProxies(cfg.TrustedProxies)),
		events:     broker,
		commentary: commentaryStore,
		uploadsDir: cfg.UploadsDir,
	}
	h.home.SetWidgets(reg.Widgets)
	return h
}

// purgeExpired removes items trashed more than trashRetentionDays ago from
// the shared lists and every user's lists, with their commentary.
func (s appServices) purgeExpired(database *sql.DB, comments *commentary.Store) {
	type purger interface {
		PurgeExpired(days int) ([]string, error)
	}
	purge := func(svc purger, attrs ...any) {
		ids, err := svc.PurgeExpired(trashRetentionDays)
		if err != nil {
			slog.Error("purge failed", append(attrs, "error", err)...)
			return
		}
		comments.ForgetItems(ids...)
	}
	purge(s.registry.Family(), "list", "family")
	purge(s.registry.HouseProjects(), "list", "house projects")
	purge(s.registry.Maintenance(), "list", "maintenance")
	users, err := auth.AllUsers(database)
	if err != nil {
		slog.Error("listing users for purge", "error", err)
		return
	}
	for _, u := range users {
		svc := s.registry.ForUser(u.ID)
		purge(svc.Personal, "list", "personal", "user_id", u.ID)
		purge(svc.Ideas, "list", "ideas", "user_id", u.ID)
	}
}

// newSessionManager builds the session manager and starts hourly cleanup of
// expired sessions.
func newSessionManager(shutdownCtx context.Context, cfg *config.Config, database *sql.DB) *scs.SessionManager {
	store := auth.NewSQLiteStore(database)
	sm := scs.New()
	sm.Store = store
	sm.Lifetime = cfg.SessionLifetime
	sm.IdleTimeout = sessionIdleTimeout
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.SecureCookies
	sm.Cookie.Name = "session"
	go runHourly(shutdownCtx, func() {
		if err := store.CleanupExpired(); err != nil {
			slog.Error("session cleanup failed", "error", err)
		}
	})
	return sm
}

// runHourly calls fn every hour until ctx is cancelled.
func runHourly(ctx context.Context, fn func()) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fn()
		case <-ctx.Done():
			return
		}
	}
}
