package commentary

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/markdown"
)

// WebGetCommentary handles GET /commentary/{list}/{id} and returns an HTML
// fragment for htmx lazy-loading. Returns empty body if no commentary exists.
func WebGetCommentary(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := chi.URLParam(r, "list")
		id := chi.URLParam(r, "id")

		if !httputil.ValidateListWithIdeas(list) {
			http.Error(w, "invalid list", http.StatusBadRequest)
			return
		}
		content, err := store.Get(id, int(auth.UserID(r.Context())))
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
		var b bytes.Buffer
		b.WriteString(`<div class="commentary-note"><span class="commentary-label">ironclaw</span>`)
		b.Write(rendered)
		b.WriteString(`</div>`)
		if _, err := w.Write(b.Bytes()); err != nil {
			slog.Debug("writing commentary", "error", err)
		}
	}
}
