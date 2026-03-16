package ideas

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/markdown"
	"github.com/fahad/dashboard/internal/slug"
)

// ToTaskFunc converts an idea to a task. Accepts context for user resolution.
type ToTaskFunc func(ctx context.Context, title, body string, tags []string) error

// ServiceResolver returns the ideas service for the current request.
type ServiceResolver func(r *http.Request) *Service

type Handler struct {
	resolve   ServiceResolver
	toTask    ToTaskFunc
	templates map[string]*template.Template
}

func NewHandler(svc *Service, toTask ToTaskFunc, templates map[string]*template.Template) *Handler {
	return &Handler{
		resolve: func(r *http.Request) *Service {
			return svc
		},
		toTask:    toTask,
		templates: templates,
	}
}

// NewHandlerWithResolver creates a handler that resolves the service per-request.
func NewHandlerWithResolver(resolver ServiceResolver, toTask ToTaskFunc, templates map[string]*template.Template) *Handler {
	return &Handler{
		resolve:   resolver,
		toTask:    toTask,
		templates: templates,
	}
}

func (h *Handler) IdeasPage(w http.ResponseWriter, r *http.Request) {
	svc := h.resolve(r)
	ideas, err := svc.List()
	if err != nil {
		slog.Error("listing ideas", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	grouped := map[string][]Idea{
		"untriaged": {},
		"parked":    {},
		"dropped":   {},
	}
	for _, idea := range ideas {
		grouped[idea.Status] = append(grouped[idea.Status], idea)
	}

	userName := auth.UserName(r.Context())
	data := map[string]any{
		"Title":     "Ideas",
		"Untriaged": grouped["untriaged"],
		"Parked":    grouped["parked"],
		"Dropped":   grouped["dropped"],
		"UserName":  userName,
		"IsAdmin":   auth.IsAdmin(r.Context()),
	}
	if userName != "" {
		data["Subtitle"] = userName + "'s ideas"
	}

	if err := h.templates["ideas.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering ideas", "error", err)
	}
}

func (h *Handler) IdeaDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc := h.resolve(r)
	idea, err := svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	bodyHTML, _ := markdown.Render([]byte(idea.Body))

	var researchHTML []byte
	if researchData, err := svc.GetResearch(slug); err == nil {
		researchHTML, _ = markdown.Render(researchData)
	}

	data := map[string]any{
		"Title":        idea.Title,
		"Idea":         idea,
		"BodyHTML":     template.HTML(bodyHTML),
		"ResearchHTML": template.HTML(researchHTML),
		"UserName":     auth.UserName(r.Context()),
		"IsAdmin":      auth.IsAdmin(r.Context()),
	}

	if err := h.templates["idea.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering idea detail", "error", err)
	}
}

func (h *Handler) QuickAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	raw := strings.TrimSpace(r.FormValue("title"))
	if raw == "" {
		http.Error(w, "Title required", http.StatusBadRequest)
		return
	}

	title, tags := parseTagsFromInput(raw)
	slug := slugify(title)
	idea := &Idea{
		Slug:   slug,
		Title:  title,
		Tags:   tags,
		Images: parseCSV(r.FormValue("images")),
		Date:   time.Now().Format("2006-01-02"),
		Body:   "# " + title + "\n\n" + strings.TrimSpace(r.FormValue("body")),
	}

	svc := h.resolve(r)
	if err := svc.Add(idea); err != nil {
		slog.Error("adding idea", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/ideas", http.StatusSeeOther)
}

func (h *Handler) TriageAction(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	svc := h.resolve(r)

	if err := svc.Triage(slug, action); err != nil {
		slog.Error("triaging idea", "slug", slug, "action", action, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas", http.StatusSeeOther)
}

func (h *Handler) ToTask(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	svc := h.resolve(r)
	idea, err := svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	body := idea.Body
	if lines := strings.SplitN(body, "\n", 2); len(lines) > 0 {
		if strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
			if len(lines) > 1 {
				body = strings.TrimSpace(lines[1])
			} else {
				body = ""
			}
		}
	}

	if err := h.toTask(r.Context(), idea.Title, body, idea.Tags); err != nil {
		slog.Error("converting idea to task", "slug", slug, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_ = svc.Delete(slug)

	http.Redirect(w, r, "/ideas", http.StatusSeeOther)
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	tags := parseCSV(r.FormValue("tags"))
	images := parseCSV(r.FormValue("images"))

	svc := h.resolve(r)
	if err := svc.Edit(slug, body, tags, images); err != nil {
		slog.Error("editing idea", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas/"+slug, http.StatusSeeOther)
}

func (h *Handler) DeleteIdea(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc := h.resolve(r)
	if err := svc.Delete(slug); err != nil {
		slog.Error("deleting idea", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas", http.StatusSeeOther)
}

// --- API handlers ---

func (h *Handler) APIListIdeas(w http.ResponseWriter, r *http.Request) {
	svc := h.resolve(r)
	ideas, err := svc.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ideas)
}

func (h *Handler) APIAddIdea(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Type  string   `json:"type"` // Legacy: converted to single tag.
		Body  string   `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}

	tags := req.Tags
	if len(tags) == 0 && req.Type != "" {
		tags = []string{req.Type}
	}

	slug := slugify(req.Title)
	body := req.Body
	if body == "" {
		body = "# " + req.Title
	}

	idea := &Idea{
		Slug:  slug,
		Title: req.Title,
		Tags:  tags,
		Date:  time.Now().Format("2006-01-02"),
		Body:  body,
	}

	svc := h.resolve(r)
	if err := svc.Add(idea); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, idea)
}

func (h *Handler) APITriageIdea(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	svc := h.resolve(r)
	if err := svc.Triage(slug, req.Action); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIAddResearch(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	svc := h.resolve(r)
	if err := svc.AddResearch(slug, req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// parseTagsFromInput extracts #tag tokens from input, returning the cleaned
// title and collected tags.
func parseTagsFromInput(input string) (title string, tags []string) {
	parts := strings.Fields(input)
	var titleParts []string
	for _, p := range parts {
		if strings.HasPrefix(p, "#") && len(p) > 1 {
			tags = append(tags, p[1:])
		} else {
			titleParts = append(titleParts, p)
		}
	}
	title = strings.Join(titleParts, " ")
	return
}

func parseCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	for v := range strings.SplitSeq(raw, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func slugify(title string) string {
	return slug.Slugify(title)
}
