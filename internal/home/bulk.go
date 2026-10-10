package home

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/tracker"
)

// planCountFlash holds the messages for bulk plan actions: the number of
// tasks (passed as ?n=) and "task" or "tasks".
var planCountFlash = map[string]string{
	"plan-bulk-completed": "%d %s done.",
	"plan-bulk-tomorrow":  "%d %s moved to tomorrow.",
	"plan-bulk-cleared":   "%d %s removed from the plan.",
	"plan-bulk-deleted":   "%d %s moved to trash.",
}

// planFlashErrorKeys are the plan flash keys shown as errors.
var planFlashErrorKeys = map[string]bool{
	"plan-bulk-failed": true,
}

// resolvePlanCountFlash fills in a counted message, or returns "" for an
// unknown key or a count that is not a small positive number.
func resolvePlanCountFlash(key, n string) string {
	format, ok := planCountFlash[key]
	if !ok {
		return ""
	}
	count, err := strconv.Atoi(n)
	if err != nil || count < 1 || count > 999 {
		return ""
	}
	tasks := "tasks"
	if count == 1 {
		tasks = "task"
	}
	return fmt.Sprintf(format, count, tasks)
}

// UncompletePlanned handles POST /plan/{id}/uncomplete: reopens a done task
// from the plan view. It keeps the planned date and plan order.
func (h *Handler) UncompletePlanned(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	svc := h.serviceForList(r, strings.TrimSpace(r.FormValue("list")))
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}
	if err := svc.Uncomplete(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "Item not found", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/?msg=plan-uncompleted", http.StatusSeeOther)
}

// planSelection is a bulk selection from the plan: IDs grouped by list, in
// the order the lists first appear.
type planSelection struct {
	order []string
	ids   map[string][]string
	svcs  map[string]*tracker.Service
	count int
}

// parsePlanSelection reads items=list:id,... and checks that every entry
// names a known list and an item in it that is not trashed, so a bad
// selection writes nothing.
func (h *Handler) parsePlanSelection(r *http.Request) (planSelection, error) {
	sel := planSelection{ids: map[string][]string{}, svcs: map[string]*tracker.Service{}}
	seen := map[string]bool{}
	for entry := range strings.SplitSeq(r.FormValue("items"), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		list, id, ok := strings.Cut(entry, ":")
		if list == "personal" {
			list = "todos"
		}
		svc := h.serviceForList(r, list)
		if !ok || svc == nil || !itemid.Valid(id) {
			return sel, fmt.Errorf("invalid selection entry")
		}
		if it, err := svc.Get(id); err != nil || it.DeletedAt != "" {
			return sel, fmt.Errorf("selected item not found")
		}
		if seen[entry] {
			continue
		}
		seen[entry] = true
		if _, ok := sel.svcs[list]; !ok {
			sel.order = append(sel.order, list)
			sel.svcs[list] = svc
		}
		sel.ids[list] = append(sel.ids[list], id)
		sel.count++
	}
	if sel.count == 0 {
		return sel, fmt.Errorf("no tasks selected")
	}
	return sel, nil
}

// bulkPlanAction returns a handler applying fn to each list's share of the
// selection, one list at a time so no two service locks are held together.
// Lists written before a failure stay written: the files cannot change
// together, so a failure reports which outcome to expect rather than
// pretending to roll back.
func (h *Handler) bulkPlanAction(msg string, fn func(svc *tracker.Service, ids []string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form data", http.StatusBadRequest)
			return
		}
		sel, err := h.parsePlanSelection(r)
		if err != nil {
			http.Error(w, "Invalid selection: "+err.Error(), http.StatusBadRequest)
			return
		}
		for _, list := range sel.order {
			if err := fn(sel.svcs[list], sel.ids[list]); err != nil {
				slog.Error("bulk plan action", "list", list, "error", err)
				http.Redirect(w, r, "/?msg=plan-bulk-failed", http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, "/?msg="+msg+"&n="+strconv.Itoa(sel.count), http.StatusSeeOther)
	}
}

// BulkCompletePlanned handles POST /plan/bulk/complete.
func (h *Handler) BulkCompletePlanned() http.HandlerFunc {
	return h.bulkPlanAction("plan-bulk-completed", (*tracker.Service).BulkComplete)
}

// BulkTomorrowPlanned handles POST /plan/bulk/tomorrow: plans the selection
// for tomorrow in the dashboard's time zone.
func (h *Handler) BulkTomorrowPlanned() http.HandlerFunc {
	return h.bulkPlanAction("plan-bulk-tomorrow", func(svc *tracker.Service, ids []string) error {
		return svc.BulkSetPlanned(ids, time.Now().In(h.loc).AddDate(0, 0, 1).Format("2006-01-02"))
	})
}

// BulkClearPlanned handles POST /plan/bulk/clear: drops the selection from
// the plan.
func (h *Handler) BulkClearPlanned() http.HandlerFunc {
	return h.bulkPlanAction("plan-bulk-cleared", (*tracker.Service).BulkClearPlanned)
}

// BulkDeletePlanned handles POST /plan/bulk/delete: moves the selection to
// trash.
func (h *Handler) BulkDeletePlanned() http.HandlerFunc {
	return h.bulkPlanAction("plan-bulk-deleted", (*tracker.Service).BulkDelete)
}
