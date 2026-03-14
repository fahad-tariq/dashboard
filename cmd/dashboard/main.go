package main

import (
	"crypto/subtle"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/projects"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/watcher"
	"github.com/fahad/dashboard/web"
)

var funcMap = template.FuncMap{
	"dict": func(pairs ...any) map[string]any {
		m := make(map[string]any, len(pairs)/2)
		for i := 0; i < len(pairs)-1; i += 2 {
			m[pairs[i].(string)] = pairs[i+1]
		}
		return m
	},
	"syncClass": func(status string) string {
		switch {
		case status == "clean":
			return "clean"
		case strings.HasPrefix(status, "ahead"):
			return "ahead"
		case strings.HasPrefix(status, "behind"):
			return "behind"
		case status == "diverged":
			return "diverged"
		default:
			return "no-remote"
		}
	},
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

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

	templates, err := parseTemplates()
	if err != nil {
		slog.Error("parsing templates", "error", err)
		os.Exit(1)
	}

	projectSvc := projects.NewService(database, cfg.ProjectsDir)
	projectStore := projects.NewStore(database)
	if err := projectSvc.Scan(); err != nil {
		slog.Error("initial project scan", "error", err)
		os.Exit(1)
	}

	ideaSvc := ideas.NewService(cfg.IdeasDir)
	broker := sse.NewBroker()

	// Start file watcher.
	if err := watcher.Watch(
		[]string{cfg.ProjectsDir, cfg.IdeasDir},
		broker,
		func() {
			if err := projectSvc.Scan(); err != nil {
				slog.Error("rescan failed", "error", err)
			}
		},
	); err != nil {
		slog.Warn("file watcher failed to start", "error", err)
	}

	projectHandler := projects.NewHandler(projectSvc, projectStore, templates)
	ideaHandler := ideas.NewHandler(ideaSvc, projectSvc, cfg.ProjectsDir, templates)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Static files.
	staticSub, _ := fs.Sub(web.StaticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// SSE endpoint for live reload.
	r.Get("/events", broker.ServeHTTP)

	// Web UI.
	r.Get("/", projectHandler.Dashboard)
	r.Post("/sync", projectHandler.SyncRefresh)
	r.Get("/projects/{slug}", projectHandler.ProjectDetail)
	r.Get("/projects/{slug}/plans/*", projectHandler.PlanView)
	r.Get("/projects/{slug}/expand", projectHandler.ExpandRow)
	r.Post("/projects/{slug}/save/{filename}", projectHandler.SaveFile)
	r.Get("/projects/{slug}/status-edit", projectHandler.StatusEdit)
	r.Put("/projects/{slug}/status", projectHandler.StatusUpdate)

	r.Get("/ideas", ideaHandler.IdeasPage)
	r.Get("/ideas/{slug}", ideaHandler.IdeaDetail)
	r.Post("/ideas/add", ideaHandler.QuickAdd)
	r.Post("/ideas/{slug}/triage", ideaHandler.TriageAction)

	// REST API with bearer token auth.
	r.Route("/api/v1", func(r chi.Router) {
		if cfg.APIToken != "" {
			r.Use(bearerAuth(cfg.APIToken))
		}
		r.Get("/projects", ideaHandler.APIListProjects)
		r.Get("/projects/{slug}", ideaHandler.APIProjectDetail)
		r.Get("/ideas", ideaHandler.APIListIdeas)
		r.Post("/ideas", ideaHandler.APIAddIdea)
		r.Put("/ideas/{slug}/triage", ideaHandler.APITriageIdea)
		r.Post("/ideas/{slug}/research", ideaHandler.APIAddResearch)
	})

	slog.Info("starting server", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// bearerAuth returns middleware that validates Bearer token on API routes.
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

// parseTemplates builds a map of page templates, each combining layout.html
// with a single page template so {{define "content"}} blocks don't collide.
func parseTemplates() (map[string]*template.Template, error) {
	layout, err := template.New("layout.html").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parsing layout: %w", err)
	}

	pages := []string{"dashboard.html", "project.html", "plan.html", "ideas.html", "idea.html"}
	templates := make(map[string]*template.Template, len(pages)+1)

	for _, page := range pages {
		t, err := template.Must(layout.Clone()).ParseFS(web.TemplateFS, "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		templates[page] = t
	}

	// Fragment templates for htmx partials.
	fragments, err := template.New("fragments").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/fragments/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing fragments: %w", err)
	}
	templates["fragments"] = fragments

	return templates, nil
}
