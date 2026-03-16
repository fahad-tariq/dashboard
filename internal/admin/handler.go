package admin

import (
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/tracker"
)

// UserStats holds per-user item counts for the admin user list.
type UserStats struct {
	PersonalTasks int
	Ideas         int
	Explorations  int
}

// Flash message keys mapped to display text.
var flashMessages = map[string]string{
	"user-created":       "User created.",
	"user-updated":       "User updated.",
	"user-deleted":       "User deleted.",
	"password-reset":     "Password reset.",
	"role-changed":       "Role updated. User sessions invalidated.",
	"cannot-delete-self":       "You cannot delete your own account.",
	"cannot-delete-last-admin": "Cannot delete the last admin.",
	"delete-failed":            "Failed to delete user.",
}

// Handler serves admin pages for user management.
type Handler struct {
	db          *sql.DB
	registry    *services.Registry
	userDataDir string
	templates   map[string]*template.Template
}

// NewHandler creates a new admin handler.
func NewHandler(db *sql.DB, registry *services.Registry, userDataDir string, templates map[string]*template.Template) *Handler {
	return &Handler{
		db:          db,
		registry:    registry,
		userDataDir: userDataDir,
		templates:   templates,
	}
}

// ListUsers renders the user list page (GET /admin/users).
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := auth.AllUsers(h.db)
	if err != nil {
		slog.Error("listing users", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Gather per-user stats when the registry is available.
	statsMap := make(map[int64]UserStats, len(users))
	if h.registry != nil {
		for _, u := range users {
			svc := h.registry.ForUser(u.ID)
			var st UserStats

			if items, err := svc.Personal.List(); err == nil {
				for _, it := range items {
					if it.Type == tracker.TaskType && !it.Done {
						st.PersonalTasks++
					}
				}
			}
			if ideasList, err := svc.Ideas.List(); err == nil {
				st.Ideas = len(ideasList)
			}
			if explorations, err := svc.Explorations.List(); err == nil {
				st.Explorations = len(explorations)
			}

			statsMap[u.ID] = st
		}
	}

	flashMsg := ""
	if key := r.URL.Query().Get("msg"); key != "" {
		flashMsg = flashMessages[key]
	}

	data := map[string]any{
		"Title":     "Admin / Users",
		"Users":     users,
		"UserStats": statsMap,
		"FlashMsg":  flashMsg,
		"UserName":  auth.UserName(r.Context()),
		"IsAdmin":   auth.IsAdmin(r.Context()),
	}

	if err := h.templates["admin-users.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering admin users", "error", err)
	}
}

// NewUserForm renders the create user form (GET /admin/users/new).
func (h *Handler) NewUserForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":    "Admin / New User",
		"FormMode": "create",
		"UserName": auth.UserName(r.Context()),
		"IsAdmin":  auth.IsAdmin(r.Context()),
	}
	if err := h.templates["admin-user-form.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering new user form", "error", err)
	}
}

// CreateUser handles the create user form submission (POST /admin/users/new).
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	role := r.FormValue("role")

	if email == "" || password == "" {
		h.renderUserForm(w, r, "create", 0, email, role, "Email and password are required.")
		return
	}
	if err := auth.ValidatePassword(password); err != nil {
		h.renderUserForm(w, r, "create", 0, email, role, err.Error())
		return
	}
	if role != "admin" && role != "user" {
		role = "user"
	}

	id, err := auth.CreateUser(h.db, email, password)
	if err != nil {
		slog.Error("creating user", "error", err)
		h.renderUserForm(w, r, "create", 0, email, role, "Failed to create user. Email may already exist.")
		return
	}

	// Override role if not the default assigned by CreateUser.
	user, _ := auth.FindByID(h.db, id)
	if user != nil && user.Role != role {
		if err := auth.UpdateUserRole(h.db, id, role); err != nil {
			slog.Error("updating role after create", "error", err)
		}
	}

	slog.Info("admin created user", "email", email, "id", id)
	http.Redirect(w, r, "/admin/users?msg=user-created", http.StatusSeeOther)
}

// EditUserForm renders the edit user form (GET /admin/users/{id}/edit).
func (h *Handler) EditUserForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user, err := auth.FindByID(h.db, id)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Title":      "Admin / Edit User",
		"FormMode":   "edit",
		"EditUser":   user,
		"EditUserID": user.ID,
		"UserName":   auth.UserName(r.Context()),
		"IsAdmin":    auth.IsAdmin(r.Context()),
	}
	if err := h.templates["admin-user-form.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering edit user form", "error", err)
	}
}

// UpdateUser handles the edit user form submission (POST /admin/users/{id}/edit).
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	user, err := auth.FindByID(h.db, id)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}

	newEmail := strings.TrimSpace(r.FormValue("email"))
	newRole := r.FormValue("role")

	if newEmail == "" {
		h.renderEditForm(w, r, user, "Email is required.")
		return
	}

	// Update email if changed.
	if newEmail != user.Email {
		if err := auth.UpdateUserEmail(h.db, id, newEmail); err != nil {
			slog.Error("updating email", "error", err)
			h.renderEditForm(w, r, user, "Failed to update email. It may already be in use.")
			return
		}
	}

	// Update role if changed.
	if newRole != "" && newRole != user.Role {
		// Prevent last admin from demoting themselves.
		if user.Role == "admin" && newRole == "user" {
			adminCount, _ := auth.AdminCount(h.db)
			if adminCount <= 1 {
				h.renderEditForm(w, r, user, "Cannot remove admin role from the last admin.")
				return
			}
		}

		if err := auth.UpdateUserRole(h.db, id, newRole); err != nil {
			slog.Error("updating role", "error", err)
			h.renderEditForm(w, r, user, "Failed to update role.")
			return
		}

		// Invalidate sessions on role change. If this fails, roll back the
		// role change to avoid a privilege mismatch with active sessions.
		if err := auth.InvalidateSessions(h.db, id); err != nil {
			slog.Error("invalidating sessions after role change", "error", err)
			if rbErr := auth.UpdateUserRole(h.db, id, user.Role); rbErr != nil {
				slog.Error("rolling back role change", "error", rbErr)
			}
			h.renderEditForm(w, r, user, "Failed to update role (session invalidation failed).")
			return
		}

		slog.Info("admin changed user role", "user_id", id, "new_role", newRole)
		http.Redirect(w, r, "/admin/users?msg=role-changed", http.StatusSeeOther)
		return
	}

	slog.Info("admin updated user", "user_id", id)
	http.Redirect(w, r, "/admin/users?msg=user-updated", http.StatusSeeOther)
}

// ResetPasswordForm renders the password reset form (GET /admin/users/{id}/password).
func (h *Handler) ResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user, err := auth.FindByID(h.db, id)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Title":      "Admin / Reset Password",
		"EditUser":   user,
		"EditUserID": user.ID,
		"UserName":   auth.UserName(r.Context()),
		"IsAdmin":    auth.IsAdmin(r.Context()),
	}
	if err := h.templates["admin-password.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering password form", "error", err)
	}
}

// ResetPassword handles the password reset form submission (POST /admin/users/{id}/password).
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	user, err := auth.FindByID(h.db, id)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if password == "" {
		h.renderPasswordForm(w, r, user, "Password is required.")
		return
	}
	if err := auth.ValidatePassword(password); err != nil {
		h.renderPasswordForm(w, r, user, err.Error())
		return
	}
	if password != confirm {
		h.renderPasswordForm(w, r, user, "Passwords do not match.")
		return
	}

	if err := auth.UpdateUserPassword(h.db, id, password); err != nil {
		slog.Error("resetting password", "error", err)
		h.renderPasswordForm(w, r, user, "Failed to reset password.")
		return
	}

	// Invalidate sessions after password reset.
	if err := auth.InvalidateSessions(h.db, id); err != nil {
		slog.Error("invalidating sessions after password reset", "error", err)
	}

	slog.Info("admin reset user password", "user_id", id)
	http.Redirect(w, r, "/admin/users?msg=password-reset", http.StatusSeeOther)
}

// DeleteUser handles user deletion (POST /admin/users/{id}/delete).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Prevent self-deletion.
	currentUserID := auth.UserID(r.Context())
	if id == currentUserID {
		http.Redirect(w, r, "/admin/users?msg=cannot-delete-self", http.StatusSeeOther)
		return
	}

	user, err := auth.FindByID(h.db, id)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}

	// Prevent deleting the last admin.
	if user.Role == "admin" {
		adminCount, _ := auth.AdminCount(h.db)
		if adminCount <= 1 {
			http.Redirect(w, r, "/admin/users?msg=cannot-delete-last-admin", http.StatusSeeOther)
			return
		}
	}

	// Cascading delete: DB rows (users, tracker_items, sessions).
	if err := auth.DeleteUser(h.db, id); err != nil {
		slog.Error("deleting user", "error", err)
		http.Redirect(w, r, "/admin/users?msg=delete-failed", http.StatusSeeOther)
		return
	}

	// Remove user data directory.
	userDir := filepath.Join(h.userDataDir, fmt.Sprintf("%d", id))
	if err := os.RemoveAll(userDir); err != nil {
		slog.Error("removing user data directory", "error", err, "path", userDir)
	}

	// Evict from service cache if registry is available.
	if h.registry != nil {
		h.registry.EvictUser(id)
	}

	slog.Info("admin deleted user", "user_id", id, "email", user.Email)
	http.Redirect(w, r, "/admin/users?msg=user-deleted", http.StatusSeeOther)
}

// --- helper renderers ---

func (h *Handler) renderUserForm(w http.ResponseWriter, r *http.Request, mode string, id int64, email, role, errMsg string) {
	data := map[string]any{
		"Title":    "Admin / " + strings.ToUpper(mode[:1]) + mode[1:] + " User",
		"FormMode": mode,
		"FormEmail": email,
		"FormRole":  role,
		"Error":    errMsg,
		"UserName": auth.UserName(r.Context()),
		"IsAdmin":  auth.IsAdmin(r.Context()),
	}
	if id != 0 {
		data["EditUserID"] = id
	}
	if err := h.templates["admin-user-form.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering user form", "error", err)
	}
}

func (h *Handler) renderEditForm(w http.ResponseWriter, r *http.Request, user *auth.User, errMsg string) {
	data := map[string]any{
		"Title":      "Admin / Edit User",
		"FormMode":   "edit",
		"EditUser":   user,
		"EditUserID": user.ID,
		"Error":      errMsg,
		"UserName":   auth.UserName(r.Context()),
		"IsAdmin":    auth.IsAdmin(r.Context()),
	}
	if err := h.templates["admin-user-form.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering edit form", "error", err)
	}
}

func (h *Handler) renderPasswordForm(w http.ResponseWriter, r *http.Request, user *auth.User, errMsg string) {
	data := map[string]any{
		"Title":      "Admin / Reset Password",
		"EditUser":   user,
		"EditUserID": user.ID,
		"Error":      errMsg,
		"UserName":   auth.UserName(r.Context()),
		"IsAdmin":    auth.IsAdmin(r.Context()),
	}
	if err := h.templates["admin-password.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering password form", "error", err)
	}
}
