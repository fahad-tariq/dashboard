// Package search serves the global search overlay from the registered
// searchers.
package search

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/web"
)

// maxResults caps the overlay's list.
const maxResults = 20

// Handler serves search requests.
type Handler struct {
	searchers []module.Searcher
	template  *template.Template
}

// NewHandler creates a search handler over searchers, queried in order.
func NewHandler(searchers []module.Searcher) *Handler {
	tmpl := template.Must(template.New("search-results.html").ParseFS(web.TemplateFS, "templates/search-results.html"))
	return &Handler{searchers: searchers, template: tmpl}
}

// SearchAPI handles GET /search?q=... and returns an HTML fragment.
func (h *Handler) SearchAPI(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || len(query) > 200 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return
	}

	// Searchers run one at a time, so no two services' locks are ever held
	// together (ToTask writes to ideas and a tracker).
	uid := auth.UserID(r.Context())
	var results []module.SearchResult
	for _, s := range h.searchers {
		results = append(results, s.Search(r.Context(), uid, query)...)
		if len(results) >= maxResults {
			results = results[:maxResults]
			break
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.template.Execute(w, map[string]any{
		"Results": results,
		"Query":   query,
	}); err != nil {
		slog.Error("rendering search results", "error", err)
	}
}

// Tracker searches one tracker list. svc returns the user's service; anchor
// prefixes the item ID in result links (e.g. "/todos#").
func Tracker(category, anchor string, svc func(userID int64) *tracker.Service) module.Searcher {
	return module.SearchFunc(func(_ context.Context, uid int64, q string) []module.SearchResult {
		var out []module.SearchResult
		for _, it := range svc(uid).Search(q) {
			out = append(out, module.SearchResult{Title: it.Title, Category: category, URL: anchor + it.ID, Snippet: Snippet(it.Body, q)})
		}
		return out
	})
}

// Maintenance searches house maintenance items.
func Maintenance(svc *house.Service) module.Searcher {
	return module.SearchFunc(func(_ context.Context, _ int64, q string) []module.SearchResult {
		var out []module.SearchResult
		for _, it := range svc.Search(q) {
			out = append(out, module.SearchResult{Title: it.Title, Category: "house", URL: "/house#maint-" + it.ID})
		}
		return out
	})
}

// Ideas searches the user's ideas.
func Ideas(svc func(userID int64) *ideas.Service) module.Searcher {
	return module.SearchFunc(func(_ context.Context, uid int64, q string) []module.SearchResult {
		var out []module.SearchResult
		for _, it := range svc(uid).Search(q) {
			out = append(out, module.SearchResult{Title: it.Title, Category: "ideas", URL: "/ideas/" + it.ID, Snippet: Snippet(it.Body, q)})
		}
		return out
	})
}

// Snippet returns a short excerpt from body around the query match.
func Snippet(body, query string) string {
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
	start := max(idx-30, 0)
	end := min(idx+len(query)+50, len(body))
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
