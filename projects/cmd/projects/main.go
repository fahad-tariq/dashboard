package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/projects/internal/config"
	"github.com/fahad/projects/internal/db"
	"github.com/fahad/projects/internal/projects"
	"github.com/fahad/projects/internal/sse"
	"github.com/fahad/projects/internal/watcher"
	"github.com/fahad/projects/web"
)

var funcMap = template.FuncMap{
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

	broker := sse.NewBroker()

	callbacks := map[string]func(){
		"projects": func() {
			if err := projectSvc.Scan(); err != nil {
				slog.Error("rescan failed", "error", err)
			}
		},
	}
	if err := watcher.Watch([]string{cfg.ProjectsDir}, broker, callbacks); err != nil {
		slog.Warn("file watcher failed to start", "error", err)
	}

	projectHandler := projects.NewHandler(projectSvc, projectStore, templates)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	staticSub, _ := fs.Sub(web.StaticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	r.Get("/events", broker.ServeHTTP)

	r.Get("/", projectHandler.Dashboard)
	r.Post("/sync", projectHandler.SyncRefresh)
	r.Get("/projects/{slug}", projectHandler.ProjectDetail)
	r.Get("/projects/{slug}/plans/*", projectHandler.PlanView)
	r.Get("/projects/{slug}/expand", projectHandler.ExpandRow)
	r.Post("/projects/{slug}/save/{filename}", projectHandler.SaveFile)
	r.Get("/projects/{slug}/status-edit", projectHandler.StatusEdit)
	r.Put("/projects/{slug}/status", projectHandler.StatusUpdate)

	slog.Info("starting server", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func parseTemplates() (map[string]*template.Template, error) {
	layout, err := template.New("layout.html").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parsing layout: %w", err)
	}

	pages := []string{"dashboard.html", "project.html", "plan.html"}
	templates := make(map[string]*template.Template, len(pages)+1)

	for _, page := range pages {
		t, err := template.Must(layout.Clone()).ParseFS(web.TemplateFS, "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		templates[page] = t
	}

	fragments, err := template.New("fragments").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/fragments/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing fragments: %w", err)
	}
	templates["fragments"] = fragments

	return templates, nil
}
