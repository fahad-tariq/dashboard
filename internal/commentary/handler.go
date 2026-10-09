package commentary

import (
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
)

const maxCommentaryLen = 5000

// APISetCommentary handles PUT /api/v1/commentary/{list}/{id}.
func APISetCommentary(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		list := chi.URLParam(r, "list")
		id := chi.URLParam(r, "id")

		if !httputil.ValidateListWithIdeas(list) {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid list"})
			return
		}
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Content == "" {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "content required"})
			return
		}
		if utf8.RuneCountInString(req.Content) > maxCommentaryLen {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "content too long (max 5000 chars)"})
			return
		}

		if err := store.Set(id, 1, req.Content); err != nil {
			httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save commentary"})
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APIGetCommentary handles GET /api/v1/commentary/{list}/{id}.
func APIGetCommentary(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := chi.URLParam(r, "list")
		id := chi.URLParam(r, "id")

		if !httputil.ValidateListWithIdeas(list) {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid list"})
			return
		}
		list = httputil.NormaliseList(list)

		content, err := store.Get(id, 1)
		if err != nil {
			httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read commentary"})
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"id":      id,
			"list":    list,
			"content": content,
		})
	}
}

// APIDeleteCommentary handles DELETE /api/v1/commentary/{list}/{id}.
func APIDeleteCommentary(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := chi.URLParam(r, "list")
		id := chi.URLParam(r, "id")

		if !httputil.ValidateListWithIdeas(list) {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid list"})
			return
		}
		if err := store.Delete(id, 1); err != nil {
			httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete commentary"})
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
