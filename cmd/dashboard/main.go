package main

import (
	"crypto/subtle"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/internal/watcher"
	"github.com/fahad/dashboard/web"
)

var funcMap = template.FuncMap{
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

	ideaSvc := ideas.NewService(cfg.IdeasDir)
	trackerStore := tracker.NewStore(database)
	trackerSvc := tracker.NewService(cfg.TrackerPath, trackerStore)
	if err := trackerSvc.Resync(); err != nil {
		slog.Warn("initial tracker sync", "error", err)
	}
	broker := sse.NewBroker()

	var watchDirs []string
	watchDirs = append(watchDirs, cfg.IdeasDir)
	if dir := filepath.Dir(cfg.TrackerPath); dir != cfg.IdeasDir {
		watchDirs = append(watchDirs, dir)
	}
	callbacks := map[string]func(){
		"tracker": func() {
			if err := trackerSvc.Resync(); err != nil {
				slog.Error("tracker resync failed", "error", err)
			}
		},
	}
	if err := watcher.Watch(watchDirs, broker, callbacks); err != nil {
		slog.Warn("file watcher failed to start", "error", err)
	}

	ideaHandler := ideas.NewHandler(ideaSvc, func(title, body, typeTag string) error {
		item := tracker.Item{
			Title: title,
			Type:  tracker.TaskType,
			Body:  body,
		}
		if typeTag != "" {
			item.Tags = []string{typeTag}
		}
		return trackerSvc.AddItem(item)
	}, templates)
	trackerHandler := tracker.NewHandler(trackerSvc, templates)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	staticSub, _ := fs.Sub(web.StaticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	r.Get("/events", broker.ServeHTTP)

	r.Get("/", trackerHandler.TrackerPage)
	r.Get("/goals", trackerHandler.GoalsPage)
	r.Post("/tracker/add", trackerHandler.QuickAdd)
	r.Post("/tracker/add-goal", trackerHandler.AddGoal)
	r.Post("/tracker/{slug}/complete", trackerHandler.Complete)
	r.Post("/tracker/{slug}/uncomplete", trackerHandler.Uncomplete)
	r.Post("/tracker/{slug}/progress", trackerHandler.UpdateProgress)
	r.Post("/tracker/{slug}/notes", trackerHandler.UpdateNotes)
	r.Post("/tracker/{slug}/delete", trackerHandler.Delete)
	r.Post("/tracker/{slug}/priority", trackerHandler.UpdatePriority)
	r.Post("/tracker/{slug}/tags", trackerHandler.UpdateTags)

	r.Get("/ideas", ideaHandler.IdeasPage)
	r.Get("/ideas/{slug}", ideaHandler.IdeaDetail)
	r.Post("/ideas/add", ideaHandler.QuickAdd)
	r.Post("/ideas/{slug}/triage", ideaHandler.TriageAction)
	r.Post("/ideas/{slug}/to-task", ideaHandler.ToTask)

	r.Route("/api/v1", func(r chi.Router) {
		if cfg.APIToken != "" {
			r.Use(bearerAuth(cfg.APIToken))
		}
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

	pages := []string{"tracker.html", "goals.html", "ideas.html", "idea.html"}
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
