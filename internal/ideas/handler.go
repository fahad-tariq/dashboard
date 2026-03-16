package ideas

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/markdown"
)

type ToTaskFunc func(title, body, typeTag string) error

type Handler struct {
	svc       *Service
	toTask    ToTaskFunc
	templates map[string]*template.Template
}

func NewHandler(svc *Service, toTask ToTaskFunc, templates map[string]*template.Template) *Handler {
	return &Handler{
		svc:       svc,
		toTask:    toTask,
		templates: templates,
	}
}

func (h *Handler) IdeasPage(w http.ResponseWriter, r *http.Request) {
	ideas, err := h.svc.List()
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

	data := map[string]any{
		"Title":     "Ideas",
		"Untriaged": grouped["untriaged"],
		"Parked":    grouped["parked"],
		"Dropped":   grouped["dropped"],
	}

	if err := h.templates["ideas.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering ideas", "error", err)
	}
}

func (h *Handler) IdeaDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	idea, err := h.svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	bodyHTML, _ := markdown.Render([]byte(idea.Body))

	var researchHTML []byte
	if researchData, err := h.svc.GetResearch(slug); err == nil {
		researchHTML, _ = markdown.Render(researchData)
	}

	data := map[string]any{
		"Title":        idea.Title,
		"Idea":         idea,
		"BodyHTML":     template.HTML(bodyHTML),
		"ResearchHTML": template.HTML(researchHTML),
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

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "Title required", http.StatusBadRequest)
		return
	}

	slug := slugify(title)
	idea := &Idea{
		Slug:  slug,
		Title: title,
		Type:  r.FormValue("type"),
		Date:  time.Now().Format("2006-01-02"),
		Body:  "# " + title + "\n\n" + strings.TrimSpace(r.FormValue("body")),
	}

	if err := h.svc.Add(idea); err != nil {
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

	if err := h.svc.Triage(slug, action); err != nil {
		slog.Error("triaging idea", "slug", slug, "action", action, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas", http.StatusSeeOther)
}

func (h *Handler) ToTask(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	idea, err := h.svc.Get(slug)
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

	typeTag := idea.Type
	if err := h.toTask(idea.Title, body, typeTag); err != nil {
		slog.Error("converting idea to task", "slug", slug, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_ = h.svc.Delete(slug)

	http.Redirect(w, r, "/ideas", http.StatusSeeOther)
}

// --- API handlers ---

func (h *Handler) APIListIdeas(w http.ResponseWriter, r *http.Request) {
	ideas, err := h.svc.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ideas)
}

func (h *Handler) APIAddIdea(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		Type  string `json:"type"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}

	slug := slugify(req.Title)
	body := req.Body
	if body == "" {
		body = "# " + req.Title
	}

	idea := &Idea{
		Slug:  slug,
		Title: req.Title,
		Type:  req.Type,
		Date:  time.Now().Format("2006-01-02"),
		Body:  body,
	}

	if err := h.svc.Add(idea); err != nil {
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

	if err := h.svc.Triage(slug, req.Action); err != nil {
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

	if err := h.svc.AddResearch(slug, req.Content); err != nil {
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

func slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
