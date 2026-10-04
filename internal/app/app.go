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

// NewRouter wires services, handlers and routes for cfg. Background goroutines
// it starts stop when shutdownCtx is cancelled; the file watcher runs for the
// life of the process.
func NewRouter(shutdownCtx context.Context, cfg *config.Config, database *sql.DB, version string) (*chi.Mux, error) {
	if err := prepareUsers(cfg, database); err != nil {
		return nil, err
	}
	assets, templates, loginTmpl, err := loadTemplates(cfg, version)
	if err != nil {
		return nil, err
	}

	broker := sse.NewBroker()
	// End open event streams as soon as shutdown starts, so http.Server.Shutdown
	// is not left waiting on them.
	context.AfterFunc(shutdownCtx, broker.Close)

	svcs, err := startServices(cfg, database, broker)
	if err != nil {
		return nil, err
	}
	go runHourly(shutdownCtx, func() { svcs.purgeExpired(database) })

	sm := newSessionManager(shutdownCtx, cfg, database)
	h := svcs.handlers(cfg, database, sm, broker, templates, loginTmpl)

	root := chi.NewRouter()
	root.Use(securityHeaders)
	root.Use(middleware.Recoverer)
	root.Use(middleware.Compress(5))

	// Every browser-facing route sits behind cross-origin protection; the
	// bearer-token API is registered on root, outside it.
	r := root.With(http.NewCrossOriginProtection().Handler)
	r.Handle("/static/*", assets.Handler())
	mountBrowserRoutes(r, cfg, database, sm, h)
	mountAPIRoutes(root, cfg, h)
	return root, nil
}

// prepareUsers creates the users start-up needs and refuses unsafe auth
// configurations.
func prepareUsers(cfg *config.Config, database *sql.DB) error {
	// Legacy password migration: auto-create admin user if DASHBOARD_PASSWORD_HASH
	// is set and no users exist in the DB.
	count, err := auth.UserCount(database)
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
	svcs.watch(cfg, broker)
	return svcs, nil
}

// watch starts the file watcher over the shared files and USER_DATA_DIR,
// plus the owner's own files when they live outside it.
func (s appServices) watch(cfg *config.Config, broker *sse.Broker) {
	fileCategories := map[string]string{
		cfg.FamilyPath:        "family",
		cfg.HouseProjectsPath: "house-projects",
		cfg.MaintenancePath:   "maintenance",
	}
	callbacks := map[string]func() bool{
		"family":         resyncCallback("family", s.registry.Family()),
		"house-projects": resyncCallback("house projects", s.registry.HouseProjects()),
		"maintenance":    resyncCallback("maintenance", s.maintenance),
	}
	if cfg.AuthDisabled {
		owner := s.registry.ForUser(ownerID)
		fileCategories[cfg.PersonalPath] = "personal"
		fileCategories[cfg.IdeasPath] = "ideas"
		callbacks["personal"] = resyncCallback("personal", owner.Personal)
		callbacks["ideas"] = resyncCallback("ideas", owner.Ideas)
	}
	userCallback := func(userID int64, category string) bool {
		if userID == 0 {
			return false
		}
		svc := s.registry.ForUser(userID)
		switch category {
		case "personal":
			return resyncCallback("personal", svc.Personal)()
		case "ideas":
			return resyncCallback("ideas", svc.Ideas)()
		}
		return false
	}
	if err := watcher.WatchWithUserCallbacks(nil, fileCategories, cfg.UserDataDir, broker, callbacks, userCallback); err != nil {
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

func (s appServices) handlers(cfg *config.Config, database *sql.DB, sm *scs.SessionManager, broker *sse.Broker, templates map[string]*template.Template, loginTmpl *template.Template) *handlers {
	commentaryStore := commentary.NewStore(database)
	h := &handlers{
		home:     home.NewHandler(s.lists, templates, cfg.Location),
		personal: tracker.NewHandlerWithResolver(s.personalAndFamily, templates, "todos", cfg.Location),
		family:   tracker.NewHandlerWithResolver(s.familyAndPersonal, templates, "family", cfg.Location),
		house:    house.NewHandler(s.maintenance, s.registry.HouseProjects(), templates, cfg.Location),
		ideas: ideas.NewHandlerWithResolver(func(r *http.Request) *ideas.Service {
			return s.registry.ForUser(auth.UserID(r.Context())).Ideas
		}, s.toTask, templates, cfg.Location),
		search: search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *tracker.Service, *house.Service, *ideas.Service) {
			l := s.lists(r)
			return l.Personal, l.Family, l.HouseProjects, l.Maintenance, l.Ideas
		}),
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
	return h
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

// resyncCallback adapts a service to the watcher: it re-reads the file only if
// it differs from the service's own last write and reports whether it did.
func resyncCallback(name string, svc interface{ ResyncIfChanged() (bool, error) }) func() bool {
	return func() bool {
		changed, err := svc.ResyncIfChanged()
		if err != nil {
			slog.Error("resync failed", "list", name, "error", err)
			return false
		}
		return changed
	}
}
