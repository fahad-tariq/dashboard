package auth

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/alexedwards/scs/v2"
)

// TemplateData returns common template fields from the request context.
func TemplateData(r *http.Request) map[string]any {
	return map[string]any{
		"UserName": UserName(r.Context()),
		"IsAdmin":  IsAdmin(r.Context()),
	}
}

type contextKey string

const (
	ctxUserID    contextKey = "user_id"
	ctxUserEmail contextKey = "user_email"
	ctxIsAdmin   contextKey = "is_admin"
	ctxFirstName contextKey = "first_name"
)

// UserID extracts the authenticated user's ID from the request context.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxUserID).(int64)
	return id
}

// UserEmail extracts the authenticated user's email from the request context.
func UserEmail(ctx context.Context) string {
	email, _ := ctx.Value(ctxUserEmail).(string)
	return email
}

// FirstName extracts the authenticated user's first name from the request context.
func FirstName(ctx context.Context) string {
	fn, _ := ctx.Value(ctxFirstName).(string)
	return fn
}

// UserName returns the user's display name. If a first name is set it takes
// priority; otherwise the local part of the email address is used.
func UserName(ctx context.Context) string {
	if fn := FirstName(ctx); fn != "" {
		return fn
	}
	return emailLocalPart(ctx)
}

// emailLocalPart returns the portion of the user's email before the @.
func emailLocalPart(ctx context.Context) string {
	email := UserEmail(ctx)
	if email == "" {
		return ""
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

// IsAdmin returns true if the current user has the admin role.
func IsAdmin(ctx context.Context) bool {
	admin, _ := ctx.Value(ctxIsAdmin).(bool)
	return admin
}

// RequireAuth redirects unauthenticated browser requests to /login,
// preserving the original path in a ?next= query parameter.
// On success, injects user_id, user_email, and is_admin into the request context.
func RequireAuth(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := sm.GetInt64(r.Context(), "user_id")
			if userID == 0 {
				dest := "/login"
				if p := r.URL.Path; p != "/" && p != "" {
					dest += "?next=" + url.QueryEscape(p)
				}
				http.Redirect(w, r, dest, http.StatusSeeOther)
				return
			}

			ctx := injectSessionContext(r.Context(), sm)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthAPI returns 401 for unauthenticated requests instead of redirecting.
// Used for SSE and other non-browser endpoints.
func RequireAuthAPI(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := sm.GetInt64(r.Context(), "user_id")
			if userID == 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}

			ctx := injectSessionContext(r.Context(), sm)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin returns 403 for non-admin users.
func RequireAdmin(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsAdmin(r.Context()) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func injectSessionContext(ctx context.Context, sm *scs.SessionManager) context.Context {
	userID := sm.GetInt64(ctx, "user_id")
	email := sm.GetString(ctx, "user_email")
	isAdmin := sm.GetBool(ctx, "is_admin")
	firstName := sm.GetString(ctx, "first_name")
	ctx = context.WithValue(ctx, ctxUserID, userID)
	ctx = context.WithValue(ctx, ctxUserEmail, email)
	ctx = context.WithValue(ctx, ctxIsAdmin, isAdmin)
	ctx = context.WithValue(ctx, ctxFirstName, firstName)
	return ctx
}

// isLocalPath validates that a next parameter is a relative path to prevent open redirects.
func isLocalPath(path string) bool {
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.HasPrefix(path, "//") {
		return false
	}
	return true
}
