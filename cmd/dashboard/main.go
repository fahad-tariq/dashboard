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
	"regexp"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/exploration"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/internal/upload"
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
	broker := sse.NewBroker()

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

	uploadHandler := upload.NewHandler(cfg.UploadsDir)

	ideaHandler := ideas.NewHandler(ideaSvc, func(title, body string, tags []string) error {
		item := tracker.Item{
			Title: title,
			Type:  tracker.TaskType,
			Body:  body,
			Tags:  tags,
		}
		return personalSvc.AddItem(item)
	}, templates)
	personalHandler := tracker.NewHandler(personalSvc, familySvc, templates, "personal")
	familyHandler := tracker.NewHandler(familySvc, personalSvc, templates, "family")
	explorationHandler := exploration.NewHandler(explorationSvc, templates)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	staticSub, _ := fs.Sub(web.StaticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	r.Post("/upload", uploadHandler.Upload)
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", noDirectoryListing(http.Dir(cfg.UploadsDir))))

	r.Get("/events", broker.ServeHTTP)

	r.Get("/", homePage(personalSvc, familySvc, ideaSvc, explorationSvc, templates))
	r.Get("/personal", personalHandler.TrackerPage)
	r.Get("/family", familyHandler.TrackerPage)
	r.Get("/goals", personalHandler.GoalsPage)

	for prefix, h := range map[string]*tracker.Handler{
		"/personal": personalHandler,
		"/family":   familyHandler,
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
	r.Post("/personal/add-goal", personalHandler.AddGoal)

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

	pages := []string{"tracker.html", "goals.html", "ideas.html", "idea.html", "exploration.html", "exploration-detail.html", "homepage.html"}
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
