package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/admin"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/exploration"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/internal/upload"
	"github.com/fahad/dashboard/internal/watcher"
	"github.com/fahad/dashboard/web"
)

var authEnabledFlag bool

var funcMap = template.FuncMap{
	"authEnabled": func() bool { return authEnabledFlag },
	"percentage": func(current, target float64) int {
		if target == 0 {
			return 0
		}
		p := int(current / target * 100)
		return max(0, min(p, 100))
	},
	"formatNum": func(f float64) string {
		if f == float64(int(f)) {
			return fmt.Sprintf("%d", int(f))
		}
		return fmt.Sprintf("%g", f)
	},
	"subtract": func(a, b int) int {
		return a - b
	},
	"linkify": func(text string) template.HTML {
		escaped := html.EscapeString(text)
		linked := urlRe.ReplaceAllStringFunc(escaped, func(u string) string {
			return `<a href="` + u + `" target="_blank" rel="noopener">` + u + `</a>`
		})
		return template.HTML(linked)
	},
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"` + "`" + `]+`)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Handle CLI subcommands before loading full config.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "useradd":
			runUserAdd()
			return
		case "migrate-data":
			runMigrateData()
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Legacy password migration: auto-create admin user if DASHBOARD_PASSWORD_HASH
	// is set and no users exist in the DB.
	count, err := auth.UserCount(database)
	if err != nil {
		slog.Error("counting users", "error", err)
		os.Exit(1)
	}
	if count == 0 && cfg.PasswordHash != "" {
		if _, err := auth.CreateUserWithHash(database, "admin@localhost", "", cfg.PasswordHash); err != nil {
			slog.Error("creating legacy admin user", "error", err)
			os.Exit(1)
		}
		slog.Info("auto-created admin@localhost from DASHBOARD_PASSWORD_HASH -- update your email with a new user")
		count = 1
	}
	cfg.HasUsers = count > 0

	templates, err := parseTemplates()
	if err != nil {
		slog.Error("parsing templates", "error", err)
		os.Exit(1)
	}

	broker := sse.NewBroker()
	uploadHandler := upload.NewHandler(cfg.UploadsDir)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Static assets are always public.
	staticSub, _ := fs.Sub(web.StaticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	authEnabledFlag = cfg.AuthEnabled()

	// ideaHandler is declared here so the API routes (below both branches)
	// can reference it regardless of which branch executes.
	var ideaHandler *ideas.Handler
	var registry *services.Registry

	if cfg.AuthEnabled() {
		// Per-user service registry: each user gets isolated personal, ideas,
		// and exploration services. Family is shared across all users.
		registry = services.NewRegistry(database, cfg.UserDataDir, cfg.FamilyPath)

		// Provision directories for every existing user on startup.
		allUsers, err := auth.AllUsers(database)
		if err != nil {
			slog.Error("loading users for directory provisioning", "error", err)
			os.Exit(1)
		}
		for _, u := range allUsers {
			if err := registry.EnsureUserDirs(u.ID); err != nil {
				slog.Error("provisioning user dirs", "user_id", u.ID, "error", err)
			}
		}

		// Initial resync for shared family service.
		if err := registry.Family().Resync(); err != nil {
			slog.Warn("initial family sync", "error", err)
		}
		// Initial resync for every known user's personal list.
		for _, u := range allUsers {
			if err := registry.ForUser(u.ID).Personal.Resync(); err != nil {
				slog.Warn("initial personal sync", "user_id", u.ID, "error", err)
			}
		}

		// File watcher: shared family file + per-user data directory.
		fileCategories := map[string]string{
			cfg.FamilyPath: "family",
		}
		callbacks := map[string]func(){
			"family": func() {
				if err := registry.Family().Resync(); err != nil {
					slog.Error("family resync failed", "error", err)
				}
			},
		}
		userCallback := func(userID int64, category string) {
			if userID == 0 || category != "personal" {
				return
			}
			svc := registry.ForUser(userID)
			if err := svc.Personal.Resync(); err != nil {
				slog.Error("per-user personal resync failed", "user_id", userID, "error", err)
			}
		}
		if err := watcher.WatchWithUserCallbacks(nil, fileCategories, cfg.UserDataDir, broker, callbacks, userCallback); err != nil {
			slog.Warn("file watcher failed to start", "error", err)
		}

		// Handlers use per-request service resolution via the registry.
		personalHandler := tracker.NewHandlerWithResolver(func(r *http.Request) (*tracker.Service, *tracker.Service) {
			uid := auth.UserID(r.Context())
			return registry.ForUser(uid).Personal, registry.Family()
		}, templates, "todos")

		familyHandler := tracker.NewHandlerWithResolver(func(r *http.Request) (*tracker.Service, *tracker.Service) {
			uid := auth.UserID(r.Context())
			return registry.Family(), registry.ForUser(uid).Personal
		}, templates, "family")

		ideaHandler = ideas.NewHandlerWithResolver(func(r *http.Request) *ideas.Service {
			return registry.ForUser(auth.UserID(r.Context())).Ideas
		}, func(ctx context.Context, title, body string, tags []string) error {
			item := tracker.Item{
				Title: title,
				Type:  tracker.TaskType,
				Body:  body,
				Tags:  tags,
			}
			return registry.ForUser(auth.UserID(ctx)).Personal.AddItem(item)
		}, templates)

		explorationHandler := exploration.NewHandlerWithResolver(func(r *http.Request) *exploration.Service {
			return registry.ForUser(auth.UserID(r.Context())).Explorations
		}, templates)

		sessionStore := auth.NewSQLiteStore(database)

		sm := scs.New()
		sm.Store = sessionStore
		sm.Lifetime = cfg.SessionLifetime
		sm.Cookie.HttpOnly = true
		sm.Cookie.SameSite = http.SameSiteLaxMode
		sm.Cookie.Secure = cfg.SecureCookies
		sm.Cookie.Name = "session"

		// Periodic cleanup of expired sessions.
		go func() {
			for {
				time.Sleep(time.Hour)
				if err := sessionStore.CleanupExpired(); err != nil {
					slog.Error("session cleanup failed", "error", err)
				}
			}
		}()

		loginTmpl, err := template.New("login.html").ParseFS(web.TemplateFS, "templates/login.html")
		if err != nil {
			slog.Error("parsing login template", "error", err)
			os.Exit(1)
		}

		limiter := auth.NewRateLimiter()
		authHandler := auth.NewHandler(sm, database, limiter, loginTmpl)

		// Public routes (no auth).
		r.Get("/login", authHandler.LoginPage)
		r.Post("/login", sm.LoadAndSave(http.HandlerFunc(authHandler.LoginSubmit)).ServeHTTP)

		// SSE: return 401 instead of redirect for unauthenticated requests.
		r.Get("/events", sm.LoadAndSave(auth.RequireAuthAPI(sm)(http.HandlerFunc(broker.ServeHTTP))).ServeHTTP)

		// Admin routes: protected by session auth + admin role.
		adminHandler := admin.NewHandler(database, registry, cfg.UserDataDir, templates)
		r.Group(func(r chi.Router) {
			r.Use(sm.LoadAndSave)
			r.Use(auth.RequireAuth(sm))
			r.Use(auth.RequireAdmin(sm))

			r.Get("/admin/users", adminHandler.ListUsers)
			r.Get("/admin/users/new", adminHandler.NewUserForm)
			r.Post("/admin/users/new", adminHandler.CreateUser)
			r.Get("/admin/users/{id}/edit", adminHandler.EditUserForm)
			r.Post("/admin/users/{id}/edit", adminHandler.UpdateUser)
			r.Get("/admin/users/{id}/password", adminHandler.ResetPasswordForm)
			r.Post("/admin/users/{id}/password", adminHandler.ResetPassword)
			r.Post("/admin/users/{id}/delete", adminHandler.DeleteUser)
		})

		// All other routes: protected by session auth.
		r.Group(func(r chi.Router) {
			r.Use(sm.LoadAndSave)
			r.Use(auth.RequireAuth(sm))

			r.Post("/logout", authHandler.Logout)
			r.Post("/upload", uploadHandler.Upload)
			r.Handle("/uploads/*", http.StripPrefix("/uploads/", noDirectoryListing(http.Dir(cfg.UploadsDir))))

			r.Get("/", homePageWithRegistry(registry, templates))
			r.Get("/todos", personalHandler.TrackerPage)
			r.Get("/personal", http.RedirectHandler("/todos", http.StatusMovedPermanently).ServeHTTP)
			r.Get("/family", familyHandler.TrackerPage)
			r.Get("/goals", personalHandler.GoalsPage)

			mountTrackerRoutes(r, personalHandler, familyHandler)

			r.Get("/ideas", ideaHandler.IdeasPage)
			r.Get("/ideas/{slug}", ideaHandler.IdeaDetail)
			r.Post("/ideas/add", ideaHandler.QuickAdd)
			r.Post("/ideas/{slug}/triage", ideaHandler.TriageAction)
			r.Post("/ideas/{slug}/to-task", ideaHandler.ToTask)
			r.Post("/ideas/{slug}/edit", ideaHandler.Edit)
			r.Post("/ideas/{slug}/delete", ideaHandler.DeleteIdea)

			r.Get("/exploration", explorationHandler.ExplorationsPage)
			r.Get("/exploration/{slug}", explorationHandler.ExplorationDetail)
			r.Post("/exploration/add", explorationHandler.QuickAdd)
			r.Post("/exploration/{slug}/edit", explorationHandler.Edit)
			r.Post("/exploration/{slug}/delete", explorationHandler.Delete)

			// Self-service account settings.
			r.Get("/account", accountPage(database, templates))
			r.Post("/account/name", accountNameSubmit(database, sm, templates))
			r.Get("/account/password", http.RedirectHandler("/account", http.StatusMovedPermanently).ServeHTTP)
			r.Post("/account/password", accountPasswordSubmit(database, templates))
		})
	} else {
		// Auth disabled: singleton services are fine for single-user mode.
		ideaSvc := ideas.NewService(cfg.IdeasDir)
		explorationSvc := exploration.NewService(cfg.ExplorationDir)
		personalStore := tracker.NewStore(database, "personal")
		familyStore := tracker.NewStore(database, "family")
		personalSvc := tracker.NewService(cfg.PersonalPath, "Personal", personalStore)
		familySvc := tracker.NewService(cfg.FamilyPath, "Family", familyStore)
		if err := personalSvc.Resync(); err != nil {
			slog.Warn("initial personal sync", "error", err)
		}
		if err := familySvc.Resync(); err != nil {
			slog.Warn("initial family sync", "error", err)
		}

		dirCategories := map[string]string{
			cfg.IdeasDir:       "ideas",
			cfg.ExplorationDir: "exploration",
		}
		fileCategories := map[string]string{
			cfg.PersonalPath: "personal",
			cfg.FamilyPath:   "family",
		}
		callbacks := map[string]func(){
			"personal": func() {
				if err := personalSvc.Resync(); err != nil {
					slog.Error("personal resync failed", "error", err)
				}
			},
			"family": func() {
				if err := familySvc.Resync(); err != nil {
					slog.Error("family resync failed", "error", err)
				}
			},
		}
		if err := watcher.Watch(dirCategories, fileCategories, broker, callbacks); err != nil {
			slog.Warn("file watcher failed to start", "error", err)
		}

		ideaHandler = ideas.NewHandler(ideaSvc, func(_ context.Context, title, body string, tags []string) error {
			item := tracker.Item{
				Title: title,
				Type:  tracker.TaskType,
				Body:  body,
				Tags:  tags,
			}
			return personalSvc.AddItem(item)
		}, templates)
		personalHandler := tracker.NewHandler(personalSvc, familySvc, templates, "todos")
		familyHandler := tracker.NewHandler(familySvc, personalSvc, templates, "family")
		explorationHandler := exploration.NewHandler(explorationSvc, templates)

		r.Post("/upload", uploadHandler.Upload)
		r.Handle("/uploads/*", http.StripPrefix("/uploads/", noDirectoryListing(http.Dir(cfg.UploadsDir))))
		r.Get("/events", broker.ServeHTTP)

		r.Get("/", homePage(personalSvc, familySvc, ideaSvc, explorationSvc, templates))
		r.Get("/todos", personalHandler.TrackerPage)
		r.Get("/personal", http.RedirectHandler("/todos", http.StatusMovedPermanently).ServeHTTP)
		r.Get("/family", familyHandler.TrackerPage)
		r.Get("/goals", personalHandler.GoalsPage)

		mountTrackerRoutes(r, personalHandler, familyHandler)

		r.Get("/ideas", ideaHandler.IdeasPage)
		r.Get("/ideas/{slug}", ideaHandler.IdeaDetail)
		r.Post("/ideas/add", ideaHandler.QuickAdd)
		r.Post("/ideas/{slug}/triage", ideaHandler.TriageAction)
		r.Post("/ideas/{slug}/to-task", ideaHandler.ToTask)
		r.Post("/ideas/{slug}/edit", ideaHandler.Edit)
		r.Post("/ideas/{slug}/delete", ideaHandler.DeleteIdea)

		r.Get("/exploration", explorationHandler.ExplorationsPage)
		r.Get("/exploration/{slug}", explorationHandler.ExplorationDetail)
		r.Post("/exploration/add", explorationHandler.QuickAdd)
		r.Post("/exploration/{slug}/edit", explorationHandler.Edit)
		r.Post("/exploration/{slug}/delete", explorationHandler.Delete)
	}

	// API routes: always use bearer token auth (separate from session auth).
	// In auth-enabled mode, API routes need a dedicated handler that doesn't
	// depend on session context for user resolution.
	apiIdeaHandler := ideaHandler
	if cfg.AuthEnabled() && apiIdeaHandler != nil {
		apiIdeaHandler = ideas.NewHandler(
			ideas.NewService(cfg.UserDataDir+"/1/ideas"),
			func(_ context.Context, title, body string, tags []string) error {
				item := tracker.Item{
					Title: title,
					Type:  tracker.TaskType,
					Body:  body,
					Tags:  tags,
				}
				return registry.ForUser(1).Personal.AddItem(item)
			},
			templates,
		)
	}
	r.Route("/api/v1", func(r chi.Router) {
		if cfg.APIToken != "" {
			r.Use(bearerAuth(cfg.APIToken))
		}
		r.Get("/ideas", apiIdeaHandler.APIListIdeas)
		r.Post("/ideas", apiIdeaHandler.APIAddIdea)
		r.Put("/ideas/{slug}/triage", apiIdeaHandler.APITriageIdea)
		r.Post("/ideas/{slug}/research", apiIdeaHandler.APIAddResearch)
	})

	slog.Info("starting server", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// runUserAdd handles the "useradd" CLI subcommand.
func runUserAdd() {
	fs := flag.NewFlagSet("useradd", flag.ExitOnError)
	email := fs.String("email", "", "user email address")
	password := fs.String("password", "", "user password")
	firstName := fs.String("first-name", "", "user first name (optional)")
	fs.Parse(os.Args[2:])

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: dashboard useradd --email <email> --password <password> [--first-name <name>]")
		os.Exit(1)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/db/dashboard.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	id, err := auth.CreateUser(database, *email, *firstName, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created user %q with id %d\n", *email, id)
}

// runMigrateData handles the "migrate-data" CLI subcommand.
func runMigrateData() {
	fs := flag.NewFlagSet("migrate-data", flag.ExitOnError)
	userID := fs.Int("user-id", 0, "target user ID")
	fs.Parse(os.Args[2:])

	if *userID <= 0 {
		fmt.Fprintln(os.Stderr, "usage: dashboard migrate-data --user-id <N>")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading config: %v\n", err)
		os.Exit(1)
	}

	userDir := fmt.Sprintf("%s/%d", cfg.UserDataDir, *userID)

	// Ensure user directories exist.
	for _, sub := range []string{"ideas/untriaged", "ideas/parked", "ideas/dropped", "ideas/research", "explorations"} {
		if err := os.MkdirAll(fmt.Sprintf("%s/%s", userDir, sub), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "creating directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Move personal.md.
	migrateFile(cfg.PersonalPath, fmt.Sprintf("%s/personal.md", userDir))

	// Move ideas.
	migrateDir(cfg.IdeasDir, fmt.Sprintf("%s/ideas", userDir))

	// Move explorations.
	migrateDir(cfg.ExplorationDir, fmt.Sprintf("%s/explorations", userDir))

	fmt.Println("data migration complete")
}

// migrateFile moves src to dst if src exists and dst does not.
func migrateFile(src, dst string) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		fmt.Printf("  skip %s (already exists at %s)\n", src, dst)
		return
	}
	if err := os.MkdirAll(strings.TrimSuffix(dst, "/"+lastPathComponent(dst)), 0o755); err != nil {
		fmt.Printf("  error creating parent dir: %v\n", err)
		return
	}

	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Printf("  error reading %s: %v\n", src, err)
		return
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		fmt.Printf("  error writing %s: %v\n", dst, err)
		return
	}
	fmt.Printf("  moved %s -> %s\n", src, dst)
}

func lastPathComponent(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// migrateDir copies files from src subdirectories to dst subdirectories.
func migrateDir(src, dst string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range entries {
		srcPath := src + "/" + entry.Name()
		dstPath := dst + "/" + entry.Name()
		if entry.IsDir() {
			migrateDir(srcPath, dstPath)
		} else {
			migrateFile(srcPath, dstPath)
		}
	}
}

// homePageWithRegistry serves the homepage using per-user services from the registry.
func homePageWithRegistry(registry *services.Registry, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UserID(r.Context())
		userSvc := registry.ForUser(uid)
		familySvc := registry.Family()

		personalSummary, _ := userSvc.Personal.Summary()
		familySummary, _ := familySvc.Summary()

		personalItems, _ := userSvc.Personal.List()
		familyItems, _ := familySvc.List()
		allIdeas, _ := userSvc.Ideas.List()
		explorations, _ := userSvc.Explorations.List()

		userName := auth.UserName(r.Context())
		data := map[string]any{
			"Title":              "Home",
			"UserName":           userName,
			"PersonalTasks":      topTasks(personalItems, 5),
			"PersonalTaskCount":  personalSummary.OpenTasks,
			"FamilyTasks":        topTasks(familyItems, 5),
			"FamilyTaskCount":    familySummary.OpenTasks,
			"Goals":              activeGoals(personalItems),
			"UntriagedIdeas":     filterUntriaged(allIdeas, 3),
			"UntriagedCount":     countUntriaged(allIdeas),
			"RecentExplorations": recentExplorations(explorations, 3),
			"ExplorationCount":   len(explorations),
			"IsAdmin":            auth.IsAdmin(r.Context()),
		}

		if err := templates["homepage.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
			slog.Error("rendering homepage", "error", err)
		}
	}
}

// homePage serves the homepage using singleton services (auth-disabled mode).
func homePage(personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, explorationSvc *exploration.Service, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSummary, _ := personalSvc.Summary()
		familySummary, _ := familySvc.Summary()

		personalItems, _ := personalSvc.List()
		familyItems, _ := familySvc.List()
		allIdeas, _ := ideaSvc.List()
		explorations, _ := explorationSvc.List()

		data := map[string]any{
			"Title":              "Home",
			"PersonalTasks":      topTasks(personalItems, 5),
			"PersonalTaskCount":  personalSummary.OpenTasks,
			"FamilyTasks":        topTasks(familyItems, 5),
			"FamilyTaskCount":    familySummary.OpenTasks,
			"Goals":              activeGoals(personalItems),
			"UntriagedIdeas":     filterUntriaged(allIdeas, 3),
			"UntriagedCount":     countUntriaged(allIdeas),
			"RecentExplorations": recentExplorations(explorations, 3),
			"ExplorationCount":   len(explorations),
		}

		if err := templates["homepage.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
			slog.Error("rendering homepage", "error", err)
		}
	}
}

// accountPasswordSubmit handles the self-service password change (POST /account/password).
func accountPasswordSubmit(database *sql.DB, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UserID(r.Context())
		user, err := auth.FindByID(database, uid)
		if err != nil || user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		password := r.FormValue("password")
		confirm := r.FormValue("confirm")

		renderErr := func(errMsg string) {
			data := map[string]any{
				"Title":         "Account Settings",
				"FirstName":     user.FirstName,
				"PasswordError": errMsg,
				"UserName":      auth.UserName(r.Context()),
				"IsAdmin":       auth.IsAdmin(r.Context()),
			}
			if err := templates["account.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
				slog.Error("rendering account page", "error", err)
			}
		}

		if password == "" {
			renderErr("Password is required.")
			return
		}
		if err := auth.ValidatePassword(password); err != nil {
			renderErr(err.Error())
			return
		}
		if password != confirm {
			renderErr("Passwords do not match.")
			return
		}

		if err := auth.UpdateUserPassword(database, uid, password); err != nil {
			slog.Error("updating own password", "error", err)
			renderErr("Failed to update password.")
			return
		}

		slog.Info("user changed own password", "user_id", uid)
		http.Redirect(w, r, "/account?msg=password-updated", http.StatusSeeOther)
	}
}

// accountPage renders the combined account settings page (GET /account).
func accountPage(database *sql.DB, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UserID(r.Context())
		user, err := auth.FindByID(database, uid)
		if err != nil || user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		var flashMsg string
		switch r.URL.Query().Get("msg") {
		case "name-updated":
			flashMsg = "Name updated."
		case "password-updated":
			flashMsg = "Password updated."
		}

		data := map[string]any{
			"Title":     "Account Settings",
			"FirstName": user.FirstName,
			"FlashMsg":  flashMsg,
			"UserName":  auth.UserName(r.Context()),
			"IsAdmin":   auth.IsAdmin(r.Context()),
		}
		if err := templates["account.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
			slog.Error("rendering account page", "error", err)
		}
	}
}

// accountNameSubmit handles the first name update (POST /account/name).
func accountNameSubmit(database *sql.DB, sm *scs.SessionManager, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UserID(r.Context())
		user, err := auth.FindByID(database, uid)
		if err != nil || user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		firstName := strings.TrimSpace(r.FormValue("first_name"))

		if err := auth.UpdateUserFirstName(database, uid, firstName); err != nil {
			slog.Error("updating first name", "user_id", uid, "error", err)
			data := map[string]any{
				"Title":     "Account Settings",
				"FirstName": firstName,
				"NameError": "Failed to update name.",
				"UserName":  auth.UserName(r.Context()),
				"IsAdmin":   auth.IsAdmin(r.Context()),
			}
			if err := templates["account.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
				slog.Error("rendering account page", "error", err)
			}
			return
		}

		// Update the session so the nav reflects the change immediately.
		sm.Put(r.Context(), "first_name", firstName)

		slog.Info("user updated first name", "user_id", uid, "first_name", firstName)
		http.Redirect(w, r, "/account?msg=name-updated", http.StatusSeeOther)
	}
}

func topTasks(items []tracker.Item, n int) []tracker.Item {
	var tasks []tracker.Item
	for _, it := range items {
		if it.Type == tracker.TaskType && !it.Done {
			tasks = append(tasks, it)
		}
	}
	slices.SortFunc(tasks, func(a, b tracker.Item) int {
		pa, pb := priorityWeight[a.Priority], priorityWeight[b.Priority]
		if pa != pb {
			return pa - pb
		}
		return 0
	})
	if len(tasks) > n {
		tasks = tasks[:n]
	}
	return tasks
}

var priorityWeight = map[string]int{"high": 0, "medium": 1, "low": 2, "": 3}

func activeGoals(items []tracker.Item) []tracker.Item {
	var goals []tracker.Item
	for _, it := range items {
		if it.Type == tracker.GoalType && !it.Done {
			goals = append(goals, it)
		}
	}
	return goals
}

func filterUntriaged(allIdeas []ideas.Idea, n int) []ideas.Idea {
	var untriaged []ideas.Idea
	for _, idea := range allIdeas {
		if idea.Status == "untriaged" {
			untriaged = append(untriaged, idea)
		}
	}
	if len(untriaged) > n {
		untriaged = untriaged[:n]
	}
	return untriaged
}

func countUntriaged(allIdeas []ideas.Idea) int {
	count := 0
	for _, idea := range allIdeas {
		if idea.Status == "untriaged" {
			count++
		}
	}
	return count
}

func recentExplorations(explorations []exploration.Exploration, n int) []exploration.Exploration {
	if len(explorations) > n {
		explorations = explorations[:n]
	}
	return explorations
}

func mountTrackerRoutes(r chi.Router, personalHandler, familyHandler *tracker.Handler) {
	for prefix, h := range map[string]*tracker.Handler{
		"/todos":  personalHandler,
		"/family": familyHandler,
	} {
		r.Post(prefix+"/add", h.QuickAdd)
		r.Post(prefix+"/{slug}/complete", h.Complete)
		r.Post(prefix+"/{slug}/uncomplete", h.Uncomplete)
		r.Post(prefix+"/{slug}/progress", h.UpdateProgress)
		r.Post(prefix+"/{slug}/notes", h.UpdateNotes)
		r.Post(prefix+"/{slug}/delete", h.Delete)
		r.Post(prefix+"/{slug}/priority", h.UpdatePriority)
		r.Post(prefix+"/{slug}/tags", h.UpdateTags)
		r.Post(prefix+"/{slug}/edit", h.UpdateEdit)
		r.Post(prefix+"/{slug}/move", h.MoveToList)
	}
	r.Post("/todos/add-goal", personalHandler.AddGoal)
}

func noDirectoryListing(root http.FileSystem) http.Handler {
	fs := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") || r.URL.Path == "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func bearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			provided, ok := strings.CutPrefix(auth, "Bearer ")
			if !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseTemplates() (map[string]*template.Template, error) {
	layout, err := template.New("layout.html").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parsing layout: %w", err)
	}

	pages := []string{"tracker.html", "goals.html", "ideas.html", "idea.html", "exploration.html", "exploration-detail.html", "homepage.html", "admin-users.html", "admin-user-form.html", "admin-password.html", "account.html"}
	templates := make(map[string]*template.Template, len(pages))

	for _, page := range pages {
		t, err := template.Must(layout.Clone()).ParseFS(web.TemplateFS, "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		templates[page] = t
	}

	return templates, nil
}
