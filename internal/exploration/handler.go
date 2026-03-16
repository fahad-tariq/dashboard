package exploration

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/markdown"
)

type Handler struct {
	svc       *Service
	templates map[string]*template.Template
}

func NewHandler(svc *Service, templates map[string]*template.Template) *Handler {
	return &Handler{svc: svc, templates: templates}
}

func (h *Handler) ExplorationsPage(w http.ResponseWriter, r *http.Request) {
	explorations, err := h.svc.List()
	if err != nil {
		slog.Error("listing explorations", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":        "Exploration",
		"Explorations": explorations,
	}

	if err := h.templates["exploration.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering explorations", "error", err)
	}
}

func (h *Handler) ExplorationDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	e, err := h.svc.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	bodyHTML, _ := markdown.Render([]byte(e.Body))

	data := map[string]any{
		"Title":       e.Title,
		"Exploration": e,
		"BodyHTML":    template.HTML(bodyHTML),
	}

	if err := h.templates["exploration-detail.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering exploration detail", "error", err)
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

	title, tags := ParseQuickAdd(raw)
	s := Slugify(title)
	e := &Exploration{
		Slug:  s,
		Title: title,
		Tags:  tags,
		Date:  time.Now().Format("2006-01-02"),
		Body:  "# " + title + "\n\n" + strings.TrimSpace(r.FormValue("body")),
	}

	if err := h.svc.Add(e); err != nil {
		slog.Error("adding exploration", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/exploration", http.StatusSeeOther)
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	tags := parseTags(r.FormValue("tags"))
	images := parseTags(r.FormValue("images"))

	if err := h.svc.Update(slug, body, tags, images); err != nil {
		slog.Error("updating exploration", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/exploration/"+slug, http.StatusSeeOther)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.svc.Delete(slug); err != nil {
		slog.Error("deleting exploration", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/exploration", http.StatusSeeOther)
}

func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var tags []string
	for t := range strings.SplitSeq(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
