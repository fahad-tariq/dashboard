package account

import (
	"database/sql"
	"errors"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/httputil"
)

var flashMessages = map[string]string{
	"name-updated":     "All good -- name saved.",
	"password-updated": "Password updated successfully.",
	"token-revoked":    "Token revoked. Anything using it now gets 401.",
}

type Handler struct {
	db        *sql.DB
	sm        *scs.SessionManager
	templates map[string]*template.Template
	loc       *time.Location
	// tokenUser is the only user whose API tokens the API accepts, so the
	// only one offered the token section.
	tokenUser int64
}

func NewHandler(db *sql.DB, sm *scs.SessionManager, templates map[string]*template.Template, loc *time.Location, tokenUser int64) *Handler {
	return &Handler{
		db:        db,
		sm:        sm,
		templates: templates,
		loc:       loc,
		tokenUser: tokenUser,
	}
}

// tokenRow is one device token as the account page lists it.
type tokenRow struct {
	ID                int64
	Name              string
	Created, LastUsed string
}

// render shows the account page with status, data merged into the base map
// and, for the token user, their device tokens.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, user *auth.User, status int, data map[string]any) {
	page := auth.TemplateData(r)
	page["Title"] = "Account Settings"
	page["FirstName"] = user.FirstName
	page["CanMintTokens"] = user.ID == h.tokenUser
	maps.Copy(page, data)
	var tokens []auth.APIToken
	if user.ID == h.tokenUser {
		var err error
		if tokens, err = auth.ListTokens(h.db, user.ID); err != nil {
			httputil.ServerError(w, "listing API tokens", err, "user_id", user.ID)
			return
		}
	}
	rows := make([]tokenRow, 0, len(tokens))
	for _, t := range tokens {
		row := tokenRow{ID: t.ID, Name: t.Name, Created: h.formatTime(t.CreatedAt), LastUsed: "never"}
		if !t.LastUsedAt.IsZero() {
			row.LastUsed = h.formatTime(t.LastUsedAt)
		}
		rows = append(rows, row)
	}
	page["Tokens"] = rows
	w.WriteHeader(status)
	if err := h.templates["account.html"].ExecuteTemplate(w, "layout.html", page); err != nil {
		slog.Error("rendering account page", "error", err)
	}
}

func (h *Handler) formatTime(t time.Time) string {
	return t.In(h.loc).Format("2 Jan 2006, 15:04")
}

func (h *Handler) currentUser(w http.ResponseWriter, r *http.Request) *auth.User {
	uid := auth.UserID(r.Context())
	user, err := auth.FindByID(h.db, uid)
	if err != nil {
		httputil.ServerError(w, "finding user", err, "user_id", uid)
		return nil
	}
	if user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return nil
	}
	return user
}

func (h *Handler) AccountPage(w http.ResponseWriter, r *http.Request) {
	user := h.currentUser(w, r)
	if user == nil {
		return
	}

	h.render(w, r, user, http.StatusOK, map[string]any{"FlashMsg": flashMessages[r.URL.Query().Get("msg")]})
}

func (h *Handler) NameSubmit(w http.ResponseWriter, r *http.Request) {
	user := h.currentUser(w, r)
	if user == nil {
		return
	}
	uid := auth.UserID(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	firstName := strings.TrimSpace(r.FormValue("first_name"))

	if err := auth.UpdateUserFirstName(h.db, uid, firstName); err != nil {
		slog.Error("updating first name", "user_id", uid, "error", err)
		h.render(w, r, user, http.StatusOK, map[string]any{"FirstName": firstName, "NameError": "Failed to update name."})
		return
	}

	h.sm.Put(r.Context(), "first_name", firstName)

	slog.Info("user updated first name", "user_id", uid, "first_name", firstName)
	http.Redirect(w, r, "/account?msg=name-updated", http.StatusSeeOther)
}

func (h *Handler) PasswordSubmit(w http.ResponseWriter, r *http.Request) {
	user := h.currentUser(w, r)
	if user == nil {
		return
	}
	uid := auth.UserID(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	renderErr := func(errMsg string) {
		h.render(w, r, user, http.StatusOK, map[string]any{"PasswordError": errMsg})
	}

	if currentPassword == "" {
		renderErr("Current password is required.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		renderErr("Current password is incorrect.")
		return
	}

	if password == "" {
		renderErr("Password is required.")
		return
	}
	if err := auth.ValidatePassword(password); err != nil {
		renderErr(err.Error())
		return
	}
	if password != confirm {
		renderErr("Passwords do not match.")
		return
	}

	if err := auth.UpdateUserPassword(h.db, uid, password); err != nil {
		slog.Error("updating own password", "error", err)
		renderErr("Failed to update password.")
		return
	}

	slog.Info("user changed own password", "user_id", uid)
	http.Redirect(w, r, "/account?msg=password-updated", http.StatusSeeOther)
}

// CreateToken mints a device token and shows it once. The page is rendered
// in reply to the POST rather than after a redirect, so the plaintext never
// passes through the session store, and it is marked no-store so no cache
// keeps it.
func (h *Handler) CreateToken(w http.ResponseWriter, r *http.Request) {
	user := h.currentUser(w, r)
	if user == nil {
		return
	}
	if user.ID != h.tokenUser {
		http.Error(w, "Only the owner's API tokens are accepted", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	plain, err := auth.CreateToken(h.db, user.ID, r.FormValue("name"))
	if errors.Is(err, auth.ErrTokenName) {
		h.render(w, r, user, http.StatusBadRequest, map[string]any{"TokenError": "Give the device a name."})
		return
	}
	if err != nil {
		httputil.ServerError(w, "creating API token", err, "user_id", user.ID)
		return
	}
	slog.Info("API token created", "user_id", user.ID)
	h.render(w, r, user, http.StatusOK, map[string]any{"NewToken": plain})
}

// RevokeToken revokes one of the session user's tokens.
func (h *Handler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	user := h.currentUser(w, r)
	if user == nil {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid token id", http.StatusBadRequest)
		return
	}
	if err := auth.RevokeToken(h.db, id, user.ID); err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) {
			http.Error(w, "Token not found", http.StatusNotFound)
			return
		}
		httputil.ServerError(w, "revoking API token", err, "user_id", user.ID)
		return
	}
	slog.Info("API token revoked", "user_id", user.ID, "token_id", id)
	http.Redirect(w, r, "/account?msg=token-revoked", http.StatusSeeOther)
}
