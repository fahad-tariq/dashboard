package ideas

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/markdown"
)

// ToTaskFunc converts an idea to a task. Accepts context for user resolution.
type ToTaskFunc func(ctx context.Context, title, body string, tags []string) error

// ServiceResolver returns the ideas service for the current request.
type ServiceResolver func(r *http.Request) *Service

var flashMessages = map[string]string{
	"title-required": "A title is required.",
	"idea-added":     "Idea captured.",
	"idea-triaged":   "Status updated.",
	"idea-edited":    "Changes saved.",
	"idea-converted": "Idea converted to a task -- check your todos.",
	"idea-deleted":   "Idea removed.",
}

var flashErrorKeys = map[string]bool{
	"title-required": true,
}

// Handler handles HTTP requests for ideas.
type Handler struct {
	resolve   ServiceResolver
	toTask    ToTaskFunc
	templates map[string]*template.Template
}

// NewHandler creates a handler with a static service reference.
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

// IdeasPage renders the ideas list grouped by status.
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
	tagSet := map[string]string{}
	for _, idea := range ideas {
		grouped[idea.Status] = append(grouped[idea.Status], idea)
		for _, t := range idea.Tags {
			tagSet[strings.ToLower(t)] = t
		}
	}
	var allTags []string
	for _, t := range tagSet {
		allTags = append(allTags, t)
	}
	slices.SortFunc(allTags, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})

	data := auth.TemplateData(r)
	data["Title"] = "Ideas"
	data["Untriaged"] = grouped["untriaged"]
	data["Parked"] = grouped["parked"]
	data["Dropped"] = grouped["dropped"]
	if msgKey := r.URL.Query().Get("msg"); msgKey != "" {
		if flashMsg := flashMessages[msgKey]; flashMsg != "" {
			data["FlashMsg"] = flashMsg
			data["FlashError"] = flashErrorKeys[msgKey]
		}
	}
	if userName := data["UserName"]; userName != "" {
		data["Subtitle"] = userName.(string) + "'s ideas"
	}

	if err := h.templates["ideas.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering ideas", "error", err)
	}
}

// IdeaDetail renders a single idea's detail page.
func (h *Handler) IdeaDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc := h.resolve(r)
	idea, err := svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	bodyHTML, _ := markdown.Render([]byte(idea.Body))

	data := auth.TemplateData(r)
	data["Title"] = idea.Title
	data["Idea"] = idea
	data["BodyHTML"] = template.HTML(bodyHTML)

	if err := h.templates["idea.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering idea detail", "error", err)
	}
}

// QuickAdd creates a new idea from the quick-add form.
func (h *Handler) QuickAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/ideas?msg=title-required", http.StatusSeeOther)
		return
	}

	idea := &Idea{
		Slug:   Slugify(title),
		Title:  title,
		Tags:   ParseCSV(r.FormValue("tags")),
		Images: ParseCSV(r.FormValue("images")),
		Added:  time.Now().Format("2006-01-02"),
		Body:   strings.TrimSpace(r.FormValue("body")),
	}

	svc := h.resolve(r)
	if err := svc.Add(idea); err != nil {
		slog.Error("adding idea", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/ideas?msg=idea-added", http.StatusSeeOther)
}

// TriageAction changes an idea's status (park/drop/untriage).
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
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas?msg=idea-triaged", http.StatusSeeOther)
}

// ToTask converts an idea to a personal task and deletes it.
func (h *Handler) ToTask(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	svc := h.resolve(r)
	idea, err := svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.toTask(r.Context(), idea.Title, idea.Body, idea.Tags); err != nil {
		slog.Error("converting idea to task", "slug", slug, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_ = svc.Delete(slug)

	http.Redirect(w, r, "/ideas?msg=idea-converted", http.StatusSeeOther)
}

// Edit updates an idea's body, tags, and images.
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tags := ParseCSV(r.FormValue("tags"))
	images := ParseCSV(r.FormValue("images"))

	svc := h.resolve(r)
	if err := svc.Edit(slug, title, body, tags, images); err != nil {
		slog.Error("editing idea", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas?msg=idea-edited", http.StatusSeeOther)
}

// DeleteIdea removes an idea.
func (h *Handler) DeleteIdea(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc := h.resolve(r)
	if err := svc.Delete(slug); err != nil {
		slog.Error("deleting idea", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/ideas?msg=idea-deleted", http.StatusSeeOther)
}

// --- API handlers ---

// APIListIdeas returns all ideas as JSON.
func (h *Handler) APIListIdeas(w http.ResponseWriter, r *http.Request) {
	svc := h.resolve(r)
	ideas, err := svc.List()
	if err != nil {
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ideas)
}

// APIAddIdea creates a new idea from a JSON request.
func (h *Handler) APIAddIdea(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Type  string   `json:"type"` // Legacy: converted to single tag.
		Body  string   `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Title == "" {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}

	tags := req.Tags
	if len(tags) == 0 && req.Type != "" {
		tags = []string{req.Type}
	}

	idea := &Idea{
		Slug:  Slugify(req.Title),
		Title: req.Title,
		Tags:  tags,
		Added: time.Now().Format("2006-01-02"),
		Body:  req.Body,
	}

	svc := h.resolve(r)
	if err := svc.Add(idea); err != nil {
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, idea)
}

// APITriageIdea changes an idea's status via JSON API.
func (h *Handler) APITriageIdea(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	slug := chi.URLParam(r, "slug")
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	svc := h.resolve(r)
	if err := svc.Triage(slug, req.Action); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// APIAddResearch appends research content to an idea's body.
func (h *Handler) APIAddResearch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	slug := chi.URLParam(r, "slug")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	svc := h.resolve(r)
	if err := svc.AddResearch(slug, req.Content); err != nil {
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}
