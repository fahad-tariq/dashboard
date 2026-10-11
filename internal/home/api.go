package home

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/tracker"
)

// apiPlan is the response of GET /api/v1/plan. Today is the server's date;
// carried-over items keep the date they were planned for.
type apiPlan struct {
	Date     string            `json:"date"`
	Today    string            `json:"today"`
	Personal []tracker.APIItem `json:"personal"`
	Family   []tracker.APIItem `json:"family"`
	House    []tracker.APIItem `json:"house"`
	Overdue  []tracker.APIItem `json:"overdue"`
}

// apiItems answers a change to several items.
type apiItems struct {
	Items []tracker.APIItem `json:"items"`
}

const listRequired = "list required (personal, family, or house)"

// APIListPlan handles GET /api/v1/plan?date=YYYY-MM-DD.
func (h *Handler) APIListPlan(w http.ResponseWriter, r *http.Request) {
	l := h.resolve(r)
	today := time.Now().In(h.loc).Format("2006-01-02")
	date := r.URL.Query().Get("date")
	if date == "" {
		date = today
	}
	if tracker.ValidDate(date) != nil {
		httputil.JSONError(w, "invalid date: use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	resp := apiPlan{Date: date, Today: today, Overdue: []tracker.APIItem{}}
	for _, pl := range planLists(l) {
		planned := tracker.ToAPIList(pl.svc.ListPlanned(date), pl.name)
		switch pl.name {
		case "personal":
			resp.Personal = planned
		case "family":
			resp.Family = planned
		case "house":
			resp.House = planned
		}
		resp.Overdue = append(resp.Overdue, tracker.ToAPIList(pl.svc.ListOverdue(date), pl.name)...)
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// APISetPlan handles PUT /api/v1/plan/{id}.
func (h *Handler) APISetPlan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	id := chi.URLParam(r, "id")
	var req struct {
		Date string `json:"date"`
		List string `json:"list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Date == "" {
		req.Date = time.Now().In(h.loc).Format("2006-01-02")
	}
	svc := h.serviceForList(r, req.List)
	if svc == nil {
		httputil.JSONError(w, listRequired, http.StatusBadRequest)
		return
	}
	if err := svc.SetPlanned(id, req.Date); err != nil {
		writePlanError(w, err)
		return
	}
	tracker.WriteStored(w, svc, id, req.List, http.StatusOK)
}

// APIClearPlan handles DELETE /api/v1/plan/{id}.
func (h *Handler) APIClearPlan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	id := chi.URLParam(r, "id")
	var req struct {
		List string `json:"list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	svc := h.serviceForList(r, req.List)
	if svc == nil {
		httputil.JSONError(w, listRequired, http.StatusBadRequest)
		return
	}
	if err := svc.ClearPlanned(id); err != nil {
		writePlanError(w, err)
		return
	}
	tracker.WriteStored(w, svc, id, req.List, http.StatusOK)
}

// APIReorderPlan handles POST /api/v1/plan/reorder and answers with the
// items in their new order.
func (h *Handler) APIReorderPlan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		IDs  []string `json:"ids"`
		List string   `json:"list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	svc := h.serviceForList(r, req.List)
	if svc == nil {
		httputil.JSONError(w, listRequired, http.StatusBadRequest)
		return
	}
	if !itemid.AllValid(req.IDs) {
		httputil.JSONError(w, "ids required: a list of item ids", http.StatusBadRequest)
		return
	}
	if err := svc.ReorderPlanned(req.IDs); err != nil {
		writePlanError(w, err)
		return
	}
	resp := apiItems{Items: make([]tracker.APIItem, 0, len(req.IDs))}
	for _, id := range req.IDs {
		it, err := svc.Get(id)
		if err != nil {
			httputil.JSONError(w, "failed to read item", http.StatusInternalServerError)
			return
		}
		resp.Items = append(resp.Items, tracker.ToAPI(*it, httputil.NormaliseList(req.List)))
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// APIClearCarried handles POST /api/v1/plan/clear-carried: it unplans every
// carried-over item in the personal, family and house lists and answers with
// them.
func (h *Handler) APIClearCarried(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, apiItems{Items: h.clearAllOverdue(h.resolve(r))})
}

func writePlanError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tracker.ErrInvalidDate):
		httputil.JSONError(w, "invalid date: use YYYY-MM-DD", http.StatusBadRequest)
	case httputil.IsNotFound(err):
		httputil.JSONError(w, "item not found", http.StatusNotFound)
	default:
		httputil.JSONError(w, "failed to update plan", http.StatusInternalServerError)
	}
}
