package home

import (
	"encoding/json"
	"net/http"

	"github.com/fahad/dashboard/internal/httputil"
)

// APIReorderPlan handles POST /api/v1/plan/reorder.
func (h *Handler) APIReorderPlan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Slugs []string `json:"slugs"`
		List  string   `json:"list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	svc := h.serviceForList(r, req.List)
	if !httputil.ValidateList(req.List) || svc == nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "list required (personal, family, or house)"})
		return
	}
	if len(req.Slugs) == 0 {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "slugs required"})
		return
	}

	if err := svc.ReorderPlanned(req.Slugs); err != nil {
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reorder"})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// APIClearCarried handles POST /api/v1/plan/clear-carried.
// Clears all overdue (carried-over) items from personal, family, and house lists.
func (h *Handler) APIClearCarried(w http.ResponseWriter, r *http.Request) {
	h.clearAllOverdue(h.resolve(r))
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
