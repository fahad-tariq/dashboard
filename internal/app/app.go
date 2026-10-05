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
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
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

// modules are the feature modules every build registers, in nav and widget
// order. Phase 7 moves the existing features here.
var modules []module.Factory

// coreNav is the core's nav: its own pages, plus the feature pages that are
// not modules yet.
var coreNav = []module.NavItem{
	{Label: "home", Path: "/", Order: 0, Group: module.Primary, Shortcut: "h"},
	{Label: "todos", Path: "/todos", Order: 10, Group: module.Primary, Shortcut: "t"},
	{Label: "goals", Path: "/goals", Order: 20, Group: module.Primary, Shortcut: "o"},
	{Label: "ideas", Path: "/ideas", Order: 30, Group: module.Primary, Shortcut: "i"},
	{Label: "house", Path: "/house", Order: 40, Group: module.Primary, Shortcut: "u"},
	{Label: "family", Path: "/family", Order: 50, Group: module.Primary, Shortcut: "f"},
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

	svcs, err := startServices(cfg, database, broker)
	if err != nil {
		return nil, err
	}
	reg, render, err := buildModules(cfg, svcs, broker, slices.Concat(modules, opts.Modules))
	if err != nil {
		return nil, err
	}
	tmpls, err := loadTemplates(cfg, version, reg)
	if err != nil {
		return nil, err
	}
	render.SetPages(tmpls.modules)
	svcs.watch(cfg, broker, reg)
	go runHourly(shutdownCtx, func() { svcs.purgeExpired(database) })

	sm := newSessionManager(shutdownCtx, cfg, database)
	h := svcs.handlers(cfg, database, sm, broker, tmpls.core, tmpls.login, reg)

	root := chi.NewRouter()
	root.Use(securityHeaders)
	root.Use(middleware.Recoverer)
	root.Use(middleware.Compress(5))

	// Every browser-facing route sits behind cross-origin protection; the
	// bearer-token API is registered on root, outside it.
	r := root.With(http.NewCrossOriginProtection().Handler)
	r.Handle("/static/*", tmpls.assets.Handler())
	mountBrowserRoutes(r, cfg, database, sm, h, reg)
	mountAPIRoutes(root, cfg, h, reg)
	return root, nil
}

// buildModules constructs the modules and validates them against the core.
// The renderer is filled once templates are parsed, which needs the nav.
func buildModules(cfg *config.Config, svcs appServices, broker *sse.Broker, factories []module.Factory) (*module.Registry, *module.Renderer, error) {
	render := &module.Renderer{}
	deps := module.Deps{
		Location: cfg.Location,
		Config:   cfg,
		Services: svcs.registry,
		Render:   render,
		DataDir:  filepath.Dir(cfg.FamilyPath),
		Publish:  broker.DebouncedChanged(publishDebounce),
	}
	mods := make([]module.Module, 0, len(factories))
	for _, f := range factories {
		mods = append(mods, f(deps))
	}
	reg, err := module.NewRegistry(module.Core{Nav: coreNav, Prefixes: []string{"/personal", "/exploration"}}, mods...)
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
	registry    *services.Registry
	maintenance *house.Service
}

// startServices builds the services, loads every known user's lists and
// starts the file watcher.
func startServices(cfg *config.Config, database *sql.DB, broker *sse.Broker) (appServices, error) {
	registry := services.NewRegistry(database, cfg.UserDataDir, cfg.FamilyPath, cfg.HouseProjectsPath, cfg.Location)
	if cfg.AuthDisabled {
		// Local development keeps PERSONAL_PATH and IDEAS_PATH.
		registry.SetUserPaths(ownerID, cfg.PersonalPath, cfg.IdeasPath)
	}
	svcs := appServices{registry: registry, maintenance: house.NewService(cfg.MaintenancePath, cfg.Location)}

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

	publish := broker.Debounced(publishDebounce)
	registry.SetPublisher(publish)
	svcs.maintenance.OnChange(func() { publish("maintenance") })
	return svcs, nil
}

// watch starts the file watcher over the core's files, the owner's files
// when they live outside USER_DATA_DIR, and every module's watched files.
// Core lists still send "file-changed" with their category until Phase 7.
func (s appServices) watch(cfg *config.Config, broker *sse.Broker, reg *module.Registry) {
	shared := func(path, category string, svc resyncer) watcher.Spec {
		return watcher.Spec{Path: path, Event: "file-changed", Data: category, Reload: func(int64) bool { return resync(category, svc) }}
	}
	specs := []watcher.Spec{
		shared(cfg.FamilyPath, "family", s.registry.Family()),
		shared(cfg.HouseProjectsPath, "house-projects", s.registry.HouseProjects()),
		shared(cfg.MaintenancePath, "maintenance", s.maintenance),
		{UserFile: "personal.md", Event: "file-changed", Data: "personal", Reload: func(uid int64) bool {
			return resync("personal", s.registry.ForUser(uid).Personal)
		}},
		{UserFile: "ideas.md", Event: "file-changed", Data: "ideas", Reload: func(uid int64) bool {
			return resync("ideas", s.registry.ForUser(uid).Ideas)
		}},
	}
	if cfg.AuthDisabled {
		owner := s.registry.ForUser(ownerID)
		specs = append(specs, shared(cfg.PersonalPath, "personal", owner.Personal), shared(cfg.IdeasPath, "ideas", owner.Ideas))
	}
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

// lists resolves the request user's services.
func (s appServices) lists(r *http.Request) home.Lists {
	u := s.registry.ForUser(auth.UserID(r.Context()))
	return home.Lists{
		Personal:      u.Personal,
		Family:        s.registry.Family(),
		HouseProjects: s.registry.HouseProjects(),
		Maintenance:   s.maintenance,
		Ideas:         u.Ideas,
	}
}

func (s appServices) personalAndFamily(r *http.Request) (*tracker.Service, *tracker.Service) {
	return s.registry.ForUser(auth.UserID(r.Context())).Personal, s.registry.Family()
}

func (s appServices) familyAndPersonal(r *http.Request) (*tracker.Service, *tracker.Service) {
	personal, family := s.personalAndFamily(r)
	return family, personal
}

// toTask adds a task converted from an idea to the target list ("personal",
// "family" or "house") and returns its slug.
func (s appServices) toTask(ctx context.Context, title, body string, tags []string, fromIdeaSlug, target string) (string, error) {
	item := tracker.Item{
		Title:    title,
		Type:     tracker.TaskType,
		Body:     body,
		Tags:     tags,
		FromIdea: fromIdeaSlug,
	}
	switch target {
	case "family":
		return s.registry.Family().AddItem(item)
	case "house":
		item.Status = "todo"
		return s.registry.HouseProjects().AddItem(item)
	default:
		return s.registry.ForUser(auth.UserID(ctx)).Personal.AddItem(item)
	}
}

func (s appServices) handlers(cfg *config.Config, database *sql.DB, sm *scs.SessionManager, broker *sse.Broker, templates map[string]*template.Template, loginTmpl *template.Template, reg *module.Registry) *handlers {
	commentaryStore := commentary.NewStore(database)
	h := &handlers{
		home:     home.NewHandler(s.lists, templates, cfg.Location),
		personal: tracker.NewHandlerWithResolver(s.personalAndFamily, templates, "todos", cfg.Location),
		family:   tracker.NewHandlerWithResolver(s.familyAndPersonal, templates, "family", cfg.Location),
		house:    house.NewHandler(s.maintenance, s.registry.HouseProjects(), templates, cfg.Location),
		ideas: ideas.NewHandlerWithResolver(func(r *http.Request) *ideas.Service {
			return s.registry.ForUser(auth.UserID(r.Context())).Ideas
		}, s.toTask, templates, cfg.Location),
		search:     search.NewHandler(slices.Concat(s.searchers(), reg.Searchers())),
		upload:     upload.NewHandler(cfg.UploadsDir),
		account:    account.NewHandler(database, sm, templates),
		admin:      admin.NewHandler(database, s.registry, cfg.UserDataDir, templates),
		auth:       auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl, auth.WithTrustedProxies(cfg.TrustedProxies)),
		events:     broker,
		commentary: commentaryStore,
		uploadsDir: cfg.UploadsDir,
		todosAPI:   s.personalAndFamily,
	}
	h.personal.SetCommentaryStore(commentaryStore)
	h.family.SetCommentaryStore(commentaryStore)
	h.ideas.SetCommentaryStore(commentaryStore)
	h.house.SetCommentaryStore(commentaryStore)
	h.home.SetWidgets(reg.Widgets)
	return h
}

// searchers search the lists that are not modules yet, in the old order.
func (s appServices) searchers() []module.Searcher {
	personal := func(uid int64) *tracker.Service { return s.registry.ForUser(uid).Personal }
	family := func(int64) *tracker.Service { return s.registry.Family() }
	houseProjects := func(int64) *tracker.Service { return s.registry.HouseProjects() }
	return []module.Searcher{
		search.Tracker("todos", "/todos#", personal),
		search.Tracker("family", "/family#", family),
		search.Tracker("house", "/house#item-", houseProjects),
		search.Maintenance(s.maintenance),
		search.Ideas(func(uid int64) *ideas.Service { return s.registry.ForUser(uid).Ideas }),
	}
}

// purgeExpired removes items trashed more than trashRetentionDays ago from
// the shared lists and every user's lists.
func (s appServices) purgeExpired(database *sql.DB) {
	type purger interface{ PurgeExpired(days int) error }
	shared := map[string]purger{
		"family":         s.registry.Family(),
		"house projects": s.registry.HouseProjects(),
		"maintenance":    s.maintenance,
	}
	for name, svc := range shared {
		if err := svc.PurgeExpired(trashRetentionDays); err != nil {
			slog.Error("purge failed", "list", name, "error", err)
		}
	}
	users, err := auth.AllUsers(database)
	if err != nil {
		slog.Error("listing users for purge", "error", err)
		return
	}
	for _, u := range users {
		svc := s.registry.ForUser(u.ID)
		if err := svc.Personal.PurgeExpired(trashRetentionDays); err != nil {
			slog.Error("personal purge failed", "user_id", u.ID, "error", err)
		}
		if err := svc.Ideas.PurgeExpired(trashRetentionDays); err != nil {
			slog.Error("ideas purge failed", "user_id", u.ID, "error", err)
		}
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

type resyncer interface{ ResyncIfChanged() (bool, error) }

// resync re-reads a service's file only if it differs from the service's own
// last write and reports whether it did.
func resync(name string, svc resyncer) bool {
	changed, err := svc.ResyncIfChanged()
	if err != nil {
		slog.Error("resync failed", "list", name, "error", err)
		return false
	}
	return changed
}
