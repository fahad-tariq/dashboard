package account

import (
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/httputil"
)

var flashMessages = map[string]string{
	"name-updated":     "Name updated.",
	"password-updated": "Password updated.",
}

type Handler struct {
	db        *sql.DB
	sm        *scs.SessionManager
	templates map[string]*template.Template
}

func NewHandler(db *sql.DB, sm *scs.SessionManager, templates map[string]*template.Template) *Handler {
	return &Handler{
		db:        db,
		sm:        sm,
		templates: templates,
	}
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

	data := auth.TemplateData(r)
	data["Title"] = "Account Settings"
	data["FirstName"] = user.FirstName
	data["FlashMsg"] = flashMessages[r.URL.Query().Get("msg")]
	if err := h.templates["account.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering account page", "error", err)
	}
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
		data := auth.TemplateData(r)
		data["Title"] = "Account Settings"
		data["FirstName"] = firstName
		data["NameError"] = "Failed to update name."
		if err := h.templates["account.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
			slog.Error("rendering account page", "error", err)
		}
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
		data := auth.TemplateData(r)
		data["Title"] = "Account Settings"
		data["FirstName"] = user.FirstName
		data["PasswordError"] = errMsg
		if err := h.templates["account.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
			slog.Error("rendering account page", "error", err)
		}
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
