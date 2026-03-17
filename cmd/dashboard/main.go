package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/account"
	"github.com/fahad/dashboard/internal/admin"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/home"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/internal/upload"
	"github.com/fahad/dashboard/internal/watcher"
	"github.com/fahad/dashboard/web"
)

var (
	version         = "dev"
	authEnabledFlag bool
)

var funcMap = template.FuncMap{
	"authEnabled": func() bool { return authEnabledFlag },
	"buildVersion": func() string { return version },
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
	"ageBadge": func(added string) []string {
		label, level := insights.AgeBadge(added, time.Now())
		return []string{label, level}
	},
	"progressColour": func(current, target float64, added, deadline string) string {
		return insights.ProgressColour(current, target, added, deadline, time.Now())
	},
	"goalPace": func(current, target float64, added, deadline string) string {
		return insights.GoalPace(current, target, added, deadline, time.Now())
	},
	"splitImageCaption": func(entry string) []string {
		file, caption := httputil.SplitImageCaption(entry)
		return []string{file, caption}
	},
	"imageFilename": func(entry string) string {
		file, _ := httputil.SplitImageCaption(entry)
		return file
	},
	"linkify": func(text string) template.HTML {
		var b strings.Builder
		last := 0
		for _, loc := range urlRe.FindAllStringIndex(text, -1) {
			b.WriteString(html.EscapeString(text[last:loc[0]]))
			rawURL := text[loc[0]:loc[1]]
			parsed, err := url.Parse(rawURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.Contains(rawURL, "'") {
				b.WriteString(html.EscapeString(rawURL))
			} else {
				b.WriteString(`<a href="`)
				b.WriteString(html.EscapeString(parsed.String()))
				b.WriteString(`" target="_blank" rel="noopener">`)
				b.WriteString(html.EscapeString(rawURL))
				b.WriteString(`</a>`)
			}
			last = loc[1]
		}
		b.WriteString(html.EscapeString(text[last:]))
		return template.HTML(b.String())
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
	r.Handle("/static/*", cacheImmutable(http.StripPrefix("/static/", http.FileServerFS(staticSub))))

	authEnabledFlag = cfg.AuthEnabled()

	// Shutdown context used by background goroutines (session cleanup, auto-purge).
	shutdownCtx, shutdownCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer shutdownCancel()

	// ideaHandler is declared here so the API routes (below both branches)
	// can reference it regardless of which branch executes.
	var ideaHandler *ideas.Handler
	var registry *services.Registry

	var personalHandler, familyHandler *tracker.Handler
	var homePage http.HandlerFunc
	var searchHandler *search.Handler
	var purgeFunc func() // called hourly to purge expired trash items

	if cfg.AuthEnabled() {
		// Per-user service registry: each user gets isolated personal and ideas
		// services. Family is shared across all users.
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
			if userID == 0 {
				return
			}
			svc := registry.ForUser(userID)
			switch category {
			case "personal":
				if err := svc.Personal.Resync(); err != nil {
					slog.Error("per-user personal resync failed", "user_id", userID, "error", err)
				}
			case "ideas":
				if err := svc.Ideas.Resync(); err != nil {
					slog.Error("per-user ideas resync failed", "user_id", userID, "error", err)
				}
			}
		}
		if err := watcher.WatchWithUserCallbacks(nil, fileCategories, cfg.UserDataDir, broker, callbacks, userCallback); err != nil {
			slog.Warn("file watcher failed to start", "error", err)
		}

		personalHandler = tracker.NewHandlerWithResolver(func(r *http.Request) (*tracker.Service, *tracker.Service) {
			uid := auth.UserID(r.Context())
			return registry.ForUser(uid).Personal, registry.Family()
		}, templates, "todos")

		familyHandler = tracker.NewHandlerWithResolver(func(r *http.Request) (*tracker.Service, *tracker.Service) {
			uid := auth.UserID(r.Context())
			return registry.Family(), registry.ForUser(uid).Personal
		}, templates, "family")

		ideaHandler = ideas.NewHandlerWithResolver(func(r *http.Request) *ideas.Service {
			return registry.ForUser(auth.UserID(r.Context())).Ideas
		}, func(ctx context.Context, title, body string, tags []string, fromIdeaSlug string) (string, error) {
			item := tracker.Item{
				Title:    title,
				Type:     tracker.TaskType,
				Body:     body,
				Tags:     tags,
				FromIdea: fromIdeaSlug,
			}
			taskSlug := tracker.Slugify(title)
			return taskSlug, registry.ForUser(auth.UserID(ctx)).Personal.AddItem(item)
		}, templates)

		searchHandler = search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *ideas.Service) {
			uid := auth.UserID(r.Context())
			svc := registry.ForUser(uid)
			return svc.Personal, registry.Family(), svc.Ideas
		})

		homeHandler := home.NewHandler(registry, templates)
		homePage = homeHandler.HomePage
		digestPage := homeHandler.DigestPage

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
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := sessionStore.CleanupExpired(); err != nil {
						slog.Error("session cleanup failed", "error", err)
					}
				case <-shutdownCtx.Done():
					return
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

		// All authenticated routes (including shared app routes).
		r.Group(func(r chi.Router) {
			r.Use(sm.LoadAndSave)
			r.Use(auth.RequireAuth(sm))

			r.Post("/logout", authHandler.Logout)

			acctHandler := account.NewHandler(database, sm, templates)
			r.Get("/account", acctHandler.AccountPage)
			r.Post("/account/name", acctHandler.NameSubmit)
			r.Get("/account/password", http.RedirectHandler("/account", http.StatusMovedPermanently).ServeHTTP)
			r.Post("/account/password", acctHandler.PasswordSubmit)

			mountAppRoutes(r, homePage, digestPage, personalHandler, familyHandler, ideaHandler, searchHandler, uploadHandler, cfg.UploadsDir)
		})

		purgeFunc = func() {
			// Purge family (shared) service.
			if err := registry.Family().PurgeExpired(7); err != nil {
				slog.Error("family purge failed", "error", err)
			}
			// Purge each user's personal and ideas services.
			allUsers, err := auth.AllUsers(database)
			if err != nil {
				slog.Error("listing users for purge", "error", err)
				return
			}
			for _, u := range allUsers {
				svc := registry.ForUser(u.ID)
				if err := svc.Personal.PurgeExpired(7); err != nil {
					slog.Error("personal purge failed", "user_id", u.ID, "error", err)
				}
				if err := svc.Ideas.PurgeExpired(7); err != nil {
					slog.Error("ideas purge failed", "user_id", u.ID, "error", err)
				}
			}
		}
	} else {
		// Auth disabled: singleton services are fine for single-user mode.
		ideaSvc := ideas.NewService(cfg.IdeasPath)
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

		fileCategories := map[string]string{
			cfg.PersonalPath: "personal",
			cfg.FamilyPath:   "family",
			cfg.IdeasPath:    "ideas",
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
			"ideas": func() {
				if err := ideaSvc.Resync(); err != nil {
					slog.Error("ideas resync failed", "error", err)
				}
			},
		}
		if err := watcher.Watch(nil, fileCategories, broker, callbacks); err != nil {
			slog.Warn("file watcher failed to start", "error", err)
		}

		ideaHandler = ideas.NewHandler(ideaSvc, func(_ context.Context, title, body string, tags []string, fromIdeaSlug string) (string, error) {
			item := tracker.Item{
				Title:    title,
				Type:     tracker.TaskType,
				Body:     body,
				Tags:     tags,
				FromIdea: fromIdeaSlug,
			}
			taskSlug := tracker.Slugify(title)
			return taskSlug, personalSvc.AddItem(item)
		}, templates)
		personalHandler = tracker.NewHandler(personalSvc, familySvc, templates, "todos")
		familyHandler = tracker.NewHandler(familySvc, personalSvc, templates, "family")
		searchHandler = search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *ideas.Service) {
			return personalSvc, familySvc, ideaSvc
		})
		homePage = home.HomePageSingle(personalSvc, familySvc, ideaSvc, templates)
		digestPage := home.DigestPageSingle(personalSvc, familySvc, ideaSvc, templates)

		r.Get("/events", broker.ServeHTTP)
		mountAppRoutes(r, homePage, digestPage, personalHandler, familyHandler, ideaHandler, searchHandler, uploadHandler, cfg.UploadsDir)

		purgeFunc = func() {
			if err := personalSvc.PurgeExpired(7); err != nil {
				slog.Error("personal purge failed", "error", err)
			}
			if err := familySvc.PurgeExpired(7); err != nil {
				slog.Error("family purge failed", "error", err)
			}
			if err := ideaSvc.PurgeExpired(7); err != nil {
				slog.Error("ideas purge failed", "error", err)
			}
		}
	}

	// Auto-purge: remove items from trash that are older than 7 days.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				purgeFunc()
			case <-shutdownCtx.Done():
				return
			}
		}
	}()

	// API routes: always use bearer token auth (separate from session auth).
	// In auth-enabled mode, API routes need a dedicated handler that doesn't
	// depend on session context for user resolution.
	apiIdeaHandler := ideaHandler
	if cfg.AuthEnabled() && apiIdeaHandler != nil {
		userSvc := registry.ForUser(1)
		apiIdeaHandler = ideas.NewHandler(
			userSvc.Ideas,
			func(_ context.Context, title, body string, tags []string, fromIdeaSlug string) (string, error) {
				item := tracker.Item{
					Title:    title,
					Type:     tracker.TaskType,
					Body:     body,
					Tags:     tags,
					FromIdea: fromIdeaSlug,
				}
				taskSlug := tracker.Slugify(title)
				return taskSlug, userSvc.Personal.AddItem(item)
			},
			templates,
		)
	}
	r.Route("/api/v1", func(r chi.Router) {
		if cfg.APIToken != "" {
			r.Use(bearerAuth(cfg.APIToken))
		} else {
			slog.Warn("API routes have no authentication -- set DASHBOARD_API_TOKEN")
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

func mountAppRoutes(r chi.Router, homePage http.HandlerFunc, digestPage http.HandlerFunc, personalHandler, familyHandler *tracker.Handler, ideaHandler *ideas.Handler, searchHandler *search.Handler, uploadHandler *upload.Handler, uploadsDir string) {
	r.Post("/upload", uploadHandler.Upload)
	r.Handle("/uploads/*", cacheImmutable(http.StripPrefix("/uploads/", noDirectoryListing(http.Dir(uploadsDir)))))

	r.Get("/search", searchHandler.SearchAPI)
	r.Get("/", homePage)
	r.Get("/digest", digestPage)
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
	r.Post("/ideas/{slug}/restore", ideaHandler.RestoreIdea)
	r.Post("/ideas/{slug}/purge", ideaHandler.PermanentDeleteIdea)
	r.Post("/ideas/bulk/delete", ideaHandler.BulkDeleteIdeas)
	r.Post("/ideas/bulk/triage", ideaHandler.BulkTriageIdeas)

	r.Get("/exploration", http.RedirectHandler("/ideas", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/exploration/{slug}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ideas/"+chi.URLParam(r, "slug"), http.StatusMovedPermanently)
	})
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
		r.Post(prefix+"/{slug}/restore", h.Restore)
		r.Post(prefix+"/{slug}/purge", h.Purge)
		r.Post(prefix+"/bulk/complete", h.BulkComplete)
		r.Post(prefix+"/bulk/delete", h.BulkDelete)
		r.Post(prefix+"/bulk/priority", h.BulkPriority)
		r.Post(prefix+"/bulk/tag", h.BulkAddTag)
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

func cacheImmutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
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

	pages := []string{"tracker.html", "goals.html", "ideas.html", "idea.html", "homepage.html", "digest.html", "admin-users.html", "admin-user-form.html", "admin-password.html", "account.html"}
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
