package projects

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/markdown"
)

type Handler struct {
	svc       *Service
	store     *Store
	templates map[string]*template.Template
}

func NewHandler(svc *Service, store *Store, templates map[string]*template.Template) *Handler {
	return &Handler{svc: svc, store: store, templates: templates}
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.List()
	if err != nil {
		slog.Error("listing projects", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":    "Dashboard",
		"Projects": projects,
	}

	if err := h.templates["dashboard.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering dashboard", "error", err)
	}
}

func (h *Handler) ProjectDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	project, err := h.svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	readmeRaw, err := os.ReadFile(filepath.Join(project.Path, "README.md"))
	if err != nil {
		readmeRaw = []byte("")
	}
	readmeHTML, err := markdown.Render(readmeRaw)
	if err != nil {
		slog.Error("rendering readme", "error", err)
		readmeHTML = readmeRaw
	}

	// Read or create backlog.md.
	backlogPath := filepath.Join(project.Path, "backlog.md")
	backlogRaw, err := os.ReadFile(backlogPath)
	if err != nil {
		backlogRaw = []byte("# Backlog\n\n## Active\n\n## Done\n")
		os.WriteFile(backlogPath, backlogRaw, 0o644)
	}

	backlogHTML, err := markdown.Render(backlogRaw)
	if err != nil {
		slog.Error("rendering backlog", "error", err)
		backlogHTML = backlogRaw
	}

	plans := ListPlans(project.Path)

	data := map[string]any{
		"Title":       project.Slug,
		"Project":     project,
		"ReadmeHTML":  template.HTML(readmeHTML),
		"ReadmeRaw":   string(readmeRaw),
		"BacklogHTML": template.HTML(backlogHTML),
		"BacklogRaw":  string(backlogRaw),
		"Plans":       plans,
	}

	if err := h.templates["project.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering project", "error", err)
	}
}

// SaveFile handles saving README.md or backlog.md.
func (h *Handler) SaveFile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "filename")

	// Only allow specific files.
	if filename != "README.md" && filename != "backlog.md" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	project, err := h.svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	content := r.FormValue("content")
	filePath := filepath.Join(project.Path, filename)

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		slog.Error("saving file", "path", filePath, "error", err)
		http.Error(w, "Failed to save", http.StatusInternalServerError)
		return
	}

	slog.Info("file saved", "project", slug, "file", filename)
	http.Redirect(w, r, "/projects/"+slug, http.StatusSeeOther)
}

func (h *Handler) PlanView(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	planPath := chi.URLParam(r, "*")

	project, err := h.svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(project.Path, filepath.Clean("/"+planPath))
	if !strings.HasPrefix(fullPath, project.Path+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	planHTML, err := markdown.Render(data)
	if err != nil {
		slog.Error("rendering plan", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	tplData := map[string]any{
		"Title":    filepath.Base(planPath),
		"Slug":     slug,
		"PlanName": filepath.Base(planPath),
		"PlanHTML": template.HTML(planHTML),
	}

	if err := h.templates["plan.html"].ExecuteTemplate(w, "layout.html", tplData); err != nil {
		slog.Error("rendering plan page", "error", err)
	}
}

func (h *Handler) ExpandRow(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.svc.GetDetail(slug)
	if err != nil {
		w.Write([]byte(`<tr class="detail-row" id="detail-` + slug + `"><td colspan="6"></td></tr>`))
		return
	}

	if err := h.templates["fragments"].ExecuteTemplate(w, "project-detail-row", detail); err != nil {
		slog.Error("rendering expand row", "error", err)
	}
}

func (h *Handler) SyncRefresh(w http.ResponseWriter, r *http.Request) {
	h.svc.FetchAllRemotes()
	h.Dashboard(w, r)
}

func (h *Handler) StatusEdit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	project, err := h.svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Slug":   project.Slug,
		"Status": project.Status,
	}

	if err := h.templates["fragments"].ExecuteTemplate(w, "status-edit", data); err != nil {
		slog.Error("rendering status edit", "error", err)
	}
}

func (h *Handler) StatusUpdate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	status := r.FormValue("status")

	if status != "active" && status != "paused" && status != "archived" {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateStatus(slug, status); err != nil {
		slog.Error("updating status", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Slug":   slug,
		"Status": status,
	}

	if err := h.templates["fragments"].ExecuteTemplate(w, "status-badge", data); err != nil {
		slog.Error("rendering status badge", "error", err)
	}
}
