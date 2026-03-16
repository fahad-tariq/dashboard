package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
)

type contextKey string

const (
	ctxUserID    contextKey = "user_id"
	ctxUserEmail contextKey = "user_email"
	ctxIsAdmin   contextKey = "is_admin"
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

// UserName returns the local part of the user's email (before the @).
func UserName(ctx context.Context) string {
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
					dest += "?next=" + p
				}
				http.Redirect(w, r, dest, http.StatusSeeOther)
				return
			}

			email := sm.GetString(r.Context(), "user_email")
			isAdmin := sm.GetBool(r.Context(), "is_admin")
			ctx := context.WithValue(r.Context(), ctxUserID, userID)
			ctx = context.WithValue(ctx, ctxUserEmail, email)
			ctx = context.WithValue(ctx, ctxIsAdmin, isAdmin)
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

			email := sm.GetString(r.Context(), "user_email")
			isAdmin := sm.GetBool(r.Context(), "is_admin")
			ctx := context.WithValue(r.Context(), ctxUserID, userID)
			ctx = context.WithValue(ctx, ctxUserEmail, email)
			ctx = context.WithValue(ctx, ctxIsAdmin, isAdmin)
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
