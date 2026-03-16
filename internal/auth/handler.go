package auth

import (
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	sm      *scs.SessionManager
	db      *sql.DB
	limiter *RateLimiter
	tmpl    *template.Template
}

func NewHandler(sm *scs.SessionManager, db *sql.DB, limiter *RateLimiter, tmpl *template.Template) *Handler {
	return &Handler{
		sm:      sm,
		db:      db,
		limiter: limiter,
		tmpl:    tmpl,
	}
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, r.URL.Query().Get("next"), "", "")
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	next := r.FormValue("next")
	email := r.FormValue("email")

	if !h.limiter.Allow(ip) {
		retryAfter := h.limiter.RetryAfter(ip)
		mins := int(retryAfter.Minutes()) + 1
		msg := fmt.Sprintf("Too many attempts. Try again in %d minute(s).", mins)
		slog.Warn("login rate limited", "ip", ip)
		h.renderLogin(w, next, msg, email)
		return
	}

	password := r.FormValue("password")

	user, err := FindByEmail(h.db, email)
	if err != nil {
		slog.Error("finding user", "error", err)
		h.renderLogin(w, next, "Internal error.", email)
		return
	}
	if user == nil {
		slog.Warn("login failed: unknown email", "ip", ip, "email", email)
		h.renderLogin(w, next, "Incorrect email or password.", email)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		slog.Warn("login failed: wrong password", "ip", ip, "email", email)
		h.renderLogin(w, next, "Incorrect email or password.", email)
		return
	}

	if err := h.sm.RenewToken(r.Context()); err != nil {
		slog.Error("renewing session token", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Store user_id in session data. The session store extracts it from the
	// gob-encoded blob during CommitCtx to tag the sessions row -- no shared
	// mutable state needed.
	h.sm.Put(r.Context(), "user_id", user.ID)
	h.sm.Put(r.Context(), "user_email", user.Email)
	h.sm.Put(r.Context(), "is_admin", user.Role == "admin")

	slog.Info("login successful", "ip", ip, "email", email)

	dest := "/"
	if isLocalPath(next) {
		dest = next
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sm.Destroy(r.Context()); err != nil {
		slog.Error("destroying session", "error", err)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) renderLogin(w http.ResponseWriter, next, errMsg, email string) {
	data := map[string]any{
		"Error": errMsg,
		"Next":  next,
		"Email": email,
	}
	if err := h.tmpl.Execute(w, data); err != nil {
		slog.Error("rendering login page", "error", err)
	}
}
