package home

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/tracker"
)

// APIReorderPlan handles POST /api/v1/plan/reorder.
func APIReorderPlan(personalSvc, familySvc, houseProjectsSvc *tracker.Service, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Slugs []string `json:"slugs"`
			List  string   `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if !httputil.ValidateList(req.List) {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "list required (personal, family, or house)"})
			return
		}
		if len(req.Slugs) == 0 {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "slugs required"})
			return
		}

		var svc *tracker.Service
		switch req.List {
		case "personal", "todos":
			svc = personalSvc
		case "family":
			svc = familySvc
		case "house":
			svc = houseProjectsSvc
		}

		if err := svc.ReorderPlanned(req.Slugs); err != nil {
			httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reorder"})
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APIClearCarried handles POST /api/v1/plan/clear-carried.
// Clears all overdue (carried-over) items from personal, family, and house lists.
func APIClearCarried(personalSvc, familySvc, houseProjectsSvc *tracker.Service, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		today := time.Now().In(loc).Format("2006-01-02")
		clearOverdue(personalSvc, today)
		clearOverdue(familySvc, today)
		clearOverdue(houseProjectsSvc, today)
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
