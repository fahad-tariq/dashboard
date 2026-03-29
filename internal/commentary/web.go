package commentary

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/markdown"
)

// WebGetCommentary handles GET /commentary/{list}/{slug} and returns an HTML
// fragment for htmx lazy-loading. Returns empty body if no commentary exists.
func WebGetCommentary(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := chi.URLParam(r, "list")
		slug := chi.URLParam(r, "slug")

		if !httputil.ValidateListWithIdeas(list) {
			http.Error(w, "invalid list", http.StatusBadRequest)
			return
		}
		list = httputil.NormaliseList(list)

		content, err := store.Get(slug, list, 1)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if content == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		rendered, err := markdown.Render([]byte(content))
		if err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<div class="commentary-note">`))
		w.Write([]byte(`<span class="commentary-label">ironclaw</span>`))
		w.Write(rendered)
		w.Write([]byte(`</div>`))
	}
}
