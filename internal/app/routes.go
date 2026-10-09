package app

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/account"
	"github.com/fahad/dashboard/internal/admin"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/commentary"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/home"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/upload"
)

// handlers holds everything the core routes call.
type handlers struct {
	home       *home.Handler
	search     *search.Handler
	upload     *upload.Handler
	account    *account.Handler
	admin      *admin.Handler
	auth       *auth.Handler
	events     *sse.Broker
	commentary *commentary.Store
	uploadsDir string
}

// mountBrowserRoutes registers the session-authenticated routes. With auth
// disabled, every request is served as the owner instead of being sent to
// /login, and the routes are otherwise identical.
func mountBrowserRoutes(r chi.Router, cfg *config.Config, database *sql.DB, sm *scs.SessionManager, h *handlers, reg *module.Registry) error {
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

	var err error
	r.Group(func(r chi.Router) {
		r.Use(sm.LoadAndSave)
		r.Use(requireUser)

		r.Post("/logout", h.auth.Logout)
		r.Get("/account", h.account.AccountPage)
		r.Post("/account/name", h.account.NameSubmit)
		r.Get("/account/password", http.RedirectHandler("/account", http.StatusMovedPermanently).ServeHTTP)
		r.Post("/account/password", h.account.PasswordSubmit)

		mountAppRoutes(r, h)
		r.Get("/commentary/{list}/"+itemid.Route, commentary.WebGetCommentary(h.commentary))
		for _, m := range reg.Modules() {
			man := m.Manifest()
			if err = mountModule(r, man.ID, man.Prefixes, m.Routes); err != nil {
				return
			}
		}
	})
	return err
}

// mountModule registers the routes register adds on r, after checking that
// each sits under one of prefixes. chi cannot list a group's routes, and lets
// a later route silently replace an earlier one, so the routes are collected
// on a scratch router and copied across with their middleware.
func mountModule(r chi.Router, id string, prefixes []string, register func(chi.Router)) error {
	scratch := chi.NewRouter()
	register(scratch)
	return chi.Walk(scratch, func(method, route string, handler http.Handler, mws ...func(http.Handler) http.Handler) error {
		if !slices.ContainsFunc(prefixes, func(p string) bool { return route == p || strings.HasPrefix(route, p+"/") }) {
			return fmt.Errorf("module %s: route %s %s is not under %v", id, method, route, prefixes)
		}
		r.With(mws...).Method(method, route, handler)
		return nil
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
	r.Post("/plan/"+itemid.Route+"/complete", h.home.CompletePlanned)
	r.Post("/plan/bulk/set", h.home.BulkSetPlanned)
	r.Post("/plan/bulk/clear-carried", h.home.ClearCarriedOver)
	r.Post("/plan/reorder", h.home.ReorderPlanned)
}

// mountAPIRoutes registers the bearer-token API, which acts as the owner
// through the same handlers and resolvers as the browser routes. Without a
// long enough DASHBOARD_API_TOKEN it is not mounted at all.
func mountAPIRoutes(root chi.Router, cfg *config.Config, h *handlers, reg *module.Registry) error {
	if len(cfg.APIToken) < minAPITokenLength {
		slog.Error("API not mounted: DASHBOARD_API_TOKEN must be set and at least 32 characters")
		return nil
	}
	var err error
	apiRateLimiter := httputil.NewRateLimiter(60, 60)
	failedBearer := auth.NewRateLimiterWithLimit(failedBearerLimit, 4096)
	root.Route("/api/v1", func(r chi.Router) {
		r.Use(bearerAuth(cfg.APIToken, failedBearer, cfg.TrustedProxies))
		r.Use(httputil.RateLimitMiddleware(apiRateLimiter))

		r.Get("/plan", h.home.APIListPlan)
		r.Put("/plan/"+itemid.Route, h.home.APISetPlan)
		r.Delete("/plan/"+itemid.Route, h.home.APIClearPlan)
		r.Post("/plan/reorder", h.home.APIReorderPlan)
		r.Post("/plan/clear-carried", h.home.APIClearCarried)
		r.Put("/commentary/{list}/"+itemid.Route, commentary.APISetCommentary(h.commentary))
		r.Get("/commentary/{list}/"+itemid.Route, commentary.APIGetCommentary(h.commentary))
		r.Delete("/commentary/{list}/"+itemid.Route, commentary.APIDeleteCommentary(h.commentary))
		// Module API routes sit under /api/v1/<module-id>, so none can
		// shadow a core API route.
		for _, m := range reg.Modules() {
			a, ok := m.(module.APIRouter)
			if !ok {
				continue
			}
			id := m.Manifest().ID
			if err = mountModule(r, id, []string{"/" + id}, a.APIRoutes); err != nil {
				return
			}
		}
	})
	return err
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
