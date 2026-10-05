package app

import (
	"crypto/subtle"
	"database/sql"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/account"
	"github.com/fahad/dashboard/internal/admin"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/commentary"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/home"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/internal/upload"
)

// handlers holds everything the routes call.
type handlers struct {
	home       *home.Handler
	personal   *tracker.Handler
	family     *tracker.Handler
	house      *house.Handler
	ideas      *ideas.Handler
	search     *search.Handler
	upload     *upload.Handler
	account    *account.Handler
	admin      *admin.Handler
	auth       *auth.Handler
	events     *sse.Broker
	commentary *commentary.Store
	uploadsDir string
	// todosAPI resolves (personal, family) for the bearer-token API.
	todosAPI tracker.ServiceResolver
}

// mountBrowserRoutes registers the session-authenticated routes. With auth
// disabled, every request is served as the owner instead of being sent to
// /login, and the routes are otherwise identical.
func mountBrowserRoutes(r chi.Router, cfg *config.Config, database *sql.DB, sm *scs.SessionManager, h *handlers, reg *module.Registry) {
	requireUser, requireUserAPI := auth.RequireAuth(sm), auth.RequireAuthAPI(sm)
	if cfg.AuthDisabled {
		requireUser = auth.InjectUser(database, ownerID)
		requireUserAPI = requireUser
	}

	r.Get("/login", h.auth.LoginPage)
	r.Post("/login", sm.LoadAndSave(http.HandlerFunc(h.auth.LoginSubmit)).ServeHTTP)

	// SSE: return 401 instead of redirect for unauthenticated requests.
	r.Get("/events", sm.LoadAndSave(requireUserAPI(http.HandlerFunc(h.events.ServeHTTP))).ServeHTTP)

	r.Group(func(r chi.Router) {
		r.Use(sm.LoadAndSave)
		r.Use(requireUser)
		r.Use(auth.RequireAdmin(sm))

		r.Get("/admin/users", h.admin.ListUsers)
		r.Get("/admin/users/new", h.admin.NewUserForm)
		r.Post("/admin/users/new", h.admin.CreateUser)
		r.Get("/admin/users/{id}/edit", h.admin.EditUserForm)
		r.Post("/admin/users/{id}/edit", h.admin.UpdateUser)
		r.Get("/admin/users/{id}/password", h.admin.ResetPasswordForm)
		r.Post("/admin/users/{id}/password", h.admin.ResetPassword)
		r.Post("/admin/users/{id}/delete", h.admin.DeleteUser)
	})

	r.Group(func(r chi.Router) {
		r.Use(sm.LoadAndSave)
		r.Use(requireUser)

		r.Post("/logout", h.auth.Logout)
		r.Get("/account", h.account.AccountPage)
		r.Post("/account/name", h.account.NameSubmit)
		r.Get("/account/password", http.RedirectHandler("/account", http.StatusMovedPermanently).ServeHTTP)
		r.Post("/account/password", h.account.PasswordSubmit)

		mountAppRoutes(r, h)
		r.Get("/commentary/{list}/{slug}", commentary.WebGetCommentary(h.commentary))
		for _, m := range reg.Modules() {
			r.Group(m.Routes)
		}
	})
}

func mountAppRoutes(r chi.Router, h *handlers) {
	r.Post("/upload", h.upload.Upload)
	r.Handle("/uploads/*", cacheImmutable(http.StripPrefix("/uploads/", noDirectoryListing(http.Dir(h.uploadsDir)))))

	r.Get("/search", h.search.SearchAPI)
	r.Get("/", h.home.HomePage)
	r.Get("/digest", h.home.DigestPage)
	r.Get("/plan/calendar", h.home.CalendarPage)

	// Daily planner routes.
	r.Post("/plan/set", h.home.SetPlanned)
	r.Post("/plan/clear", h.home.ClearPlanned)
	r.Post("/plan/{slug}/complete", h.home.CompletePlanned)
	r.Post("/plan/bulk/set", h.home.BulkSetPlanned)
	r.Post("/plan/bulk/clear-carried", h.home.ClearCarriedOver)
	r.Post("/plan/reorder", h.home.ReorderPlanned)
	r.Get("/todos", h.personal.TrackerPage)
	r.Get("/personal", http.RedirectHandler("/todos", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/family", h.family.TrackerPage)
	r.Get("/goals", h.personal.GoalsPage)

	mountTrackerRoutes(r, h.personal, h.family)

	// House page (combined maintenance + projects).
	r.Get("/house", h.house.HousePage)
	r.Post("/house/maintenance/add", h.house.AddMaintenance)
	r.Post("/house/maintenance/{slug}/log", h.house.LogDone)
	r.Post("/house/maintenance/{slug}/edit", h.house.EditMaintenance)
	r.Post("/house/maintenance/{slug}/delete", h.house.DeleteMaintenance)
	r.Post("/house/maintenance/{slug}/restore", h.house.RestoreMaintenance)
	r.Post("/house/maintenance/{slug}/purge", h.house.PurgeMaintenance)
	r.Post("/house/projects/add", h.house.AddProject)
	r.Post("/house/projects/{slug}/edit", h.house.EditProject)
	r.Post("/house/projects/{slug}/complete", h.house.CompleteProject)
	r.Post("/house/projects/{slug}/uncomplete", h.house.UncompleteProject)
	r.Post("/house/projects/{slug}/status", h.house.UpdateStatus)
	r.Post("/house/projects/{slug}/delete", h.house.DeleteProject)
	r.Post("/house/projects/{slug}/restore", h.house.RestoreProject)
	r.Post("/house/projects/{slug}/purge", h.house.PurgeProject)

	r.Get("/ideas", h.ideas.IdeasPage)
	r.Get("/ideas/{slug}", h.ideas.IdeaDetail)
	r.Post("/ideas/add", h.ideas.QuickAdd)
	r.Post("/ideas/{slug}/triage", h.ideas.TriageAction)
	r.Post("/ideas/{slug}/to-task", h.ideas.ToTask)
	r.Post("/ideas/{slug}/edit", h.ideas.Edit)
	r.Post("/ideas/{slug}/delete", h.ideas.DeleteIdea)
	r.Post("/ideas/{slug}/restore", h.ideas.RestoreIdea)
	r.Post("/ideas/{slug}/purge", h.ideas.PermanentDeleteIdea)
	r.Post("/ideas/bulk/delete", h.ideas.BulkDeleteIdeas)
	r.Post("/ideas/bulk/triage", h.ideas.BulkTriageIdeas)

	r.Get("/exploration", http.RedirectHandler("/ideas", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/exploration/{slug}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ideas/"+chi.URLParam(r, "slug"), http.StatusMovedPermanently) //nolint:gosec // G710: chi params cannot contain "/", so the target stays under /ideas/
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
		r.Post(prefix+"/{slug}/plan", h.PlanForToday)
		r.Post(prefix+"/{slug}/substep/add", h.AddSubStep)
		r.Post(prefix+"/{slug}/substep/toggle", h.ToggleSubStep)
		r.Post(prefix+"/{slug}/substep/remove", h.RemoveSubStep)
		r.Post(prefix+"/{slug}/substep/promote", h.PromoteSubStep)
		r.Post(prefix+"/bulk/plan", h.BulkPlanForToday)
	}
	r.Post("/todos/add-goal", personalHandler.AddGoal)
}

// mountAPIRoutes registers the bearer-token API, which acts as the owner
// through the same handlers and resolvers as the browser routes. Without a
// long enough DASHBOARD_API_TOKEN it is not mounted at all.
func mountAPIRoutes(root chi.Router, cfg *config.Config, h *handlers, reg *module.Registry) {
	if len(cfg.APIToken) < minAPITokenLength {
		slog.Error("API not mounted: DASHBOARD_API_TOKEN must be set and at least 32 characters")
		return
	}
	apiRateLimiter := httputil.NewRateLimiter(60, 60)
	failedBearer := auth.NewRateLimiterWithLimit(failedBearerLimit, 4096)
	root.Route("/api/v1", func(r chi.Router) {
		r.Use(bearerAuth(cfg.APIToken, failedBearer, cfg.TrustedProxies))
		r.Use(httputil.RateLimitMiddleware(apiRateLimiter))

		r.Get("/ideas", h.ideas.APIListIdeas)
		r.Post("/ideas", h.ideas.APIAddIdea)
		r.Put("/ideas/{slug}/triage", h.ideas.APITriageIdea)
		r.Post("/ideas/{slug}/research", h.ideas.APIAddResearch)
		r.Get("/plan", h.home.APIListPlan)
		r.Put("/plan/{slug}", h.home.APISetPlan)
		r.Delete("/plan/{slug}", h.home.APIClearPlan)
		r.Post("/plan/reorder", h.home.APIReorderPlan)
		r.Post("/plan/clear-carried", h.home.APIClearCarried)
		r.Put("/commentary/{list}/{slug}", commentary.APISetCommentary(h.commentary))
		r.Get("/commentary/{list}/{slug}", commentary.APIGetCommentary(h.commentary))
		r.Delete("/commentary/{list}/{slug}", commentary.APIDeleteCommentary(h.commentary))
		r.Get("/todos", tracker.APIListTodos(h.todosAPI))
		r.Post("/todos", tracker.APIAddTodo(h.todosAPI))
		r.Get("/todos/{slug}", tracker.APIGetTodo(h.todosAPI))
		r.Put("/todos/{slug}", tracker.APIUpdateTodo(h.todosAPI))
		r.Post("/todos/{slug}/complete", tracker.APICompleteTodo(h.todosAPI))
		r.Post("/todos/{slug}/uncomplete", tracker.APIUncompleteTodo(h.todosAPI))
		r.Delete("/todos/{slug}", tracker.APIDeleteTodo(h.todosAPI))
		r.Put("/todos/{slug}/priority", tracker.APIUpdatePriority(h.todosAPI))
		r.Put("/todos/{slug}/tags", tracker.APIUpdateTags(h.todosAPI))
		r.Post("/todos/{slug}/substeps", tracker.APIAddSubStep(h.todosAPI))
		r.Put("/todos/{slug}/substeps/{index}", tracker.APIToggleSubStep(h.todosAPI))
		r.Delete("/todos/{slug}/substeps/{index}", tracker.APIRemoveSubStep(h.todosAPI))
		for _, m := range reg.Modules() {
			if a, ok := m.(module.APIRouter); ok {
				r.Group(a.APIRoutes)
			}
		}
	})
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

// bearerAuth checks the API token and serves valid requests as the owner.
// Failed attempts are counted per client IP and answered with 429 past the
// limit; a valid token always gets through, so bad guesses cannot lock out a
// real client behind the same address.
func bearerAuth(token string, failures *auth.RateLimiter, trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if ok && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
				next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: ownerID})))
				return
			}
			status, body := http.StatusUnauthorized, `{"error":"unauthorized"}`
			if !failures.Allow(httputil.ClientIP(r, trusted)) {
				status, body = http.StatusTooManyRequests, `{"error":"too many failed attempts"}`
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
