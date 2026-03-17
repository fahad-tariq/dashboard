package search

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/web"
)

// SearchResult represents a single search hit across all content types.
type SearchResult struct {
	Title    string
	Slug     string
	Category string // "todos", "family", "ideas"
	URL      string
	Snippet  string
}

// ServiceResolver returns the personal, family, and ideas services for the request.
type ServiceResolver func(r *http.Request) (personal *tracker.Service, family *tracker.Service, ideaSvc *ideas.Service)

// Handler serves search requests.
type Handler struct {
	resolve  ServiceResolver
	template *template.Template
}

// NewHandler creates a search handler.
func NewHandler(resolver ServiceResolver) *Handler {
	tmpl := template.Must(template.New("search-results.html").ParseFS(web.TemplateFS, "templates/search-results.html"))
	return &Handler{
		resolve:  resolver,
		template: tmpl,
	}
}

// SearchAPI handles GET /search?q=... and returns an HTML fragment.
func (h *Handler) SearchAPI(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || len(query) > 200 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return
	}

	personal, family, ideaSvc := h.resolve(r)

	var results []SearchResult

	// Lock services sequentially, never simultaneously.
	personalItems := personal.Search(query)
	for _, it := range personalItems {
		results = append(results, SearchResult{
			Title:    it.Title,
			Slug:     it.Slug,
			Category: "todos",
			URL:      "/todos#" + it.Slug,
			Snippet:  snippet(it.Body, query),
		})
	}

	familyItems := family.Search(query)
	for _, it := range familyItems {
		results = append(results, SearchResult{
			Title:    it.Title,
			Slug:     it.Slug,
			Category: "family",
			URL:      "/family#" + it.Slug,
			Snippet:  snippet(it.Body, query),
		})
	}

	ideaItems := ideaSvc.Search(query)
	for _, it := range ideaItems {
		results = append(results, SearchResult{
			Title:    it.Title,
			Slug:     it.Slug,
			Category: "ideas",
			URL:      "/ideas/" + it.Slug,
			Snippet:  snippet(it.Body, query),
		})
	}

	// Cap results at 20.
	if len(results) > 20 {
		results = results[:20]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.template.Execute(w, map[string]any{
		"Results": results,
		"Query":   query,
	}); err != nil {
		slog.Error("rendering search results", "error", err)
	}
}

// snippet returns a short excerpt from body around the query match.
func snippet(body, query string) string {
	if body == "" {
		return ""
	}
	lower := strings.ToLower(body)
	q := strings.ToLower(query)
	idx := strings.Index(lower, q)
	if idx < 0 {
		// No match in body, return first 80 chars.
		if len(body) > 80 {
			return body[:80] + "..."
		}
		return body
	}
	start := idx - 30
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 50
	if end > len(body) {
		end = len(body)
	}
	s := body[start:end]
	// Clean up newlines.
	s = strings.ReplaceAll(s, "\n", " ")
	if start > 0 {
		s = "..." + s
	}
	if end < len(body) {
		s = s + "..."
	}
	return s
}
