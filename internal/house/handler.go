package house

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/commentary"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/tracker"
)

var flashMessages = map[string]string{
	"title-required":     "A title is required.",
	"cadence-required":   "A cadence is required (e.g. 2w, 3m).",
	"cadence-invalid":    "Invalid cadence format.",
	"item-edited":        "Changes saved.",
	"item-deleted":       "Item moved to trash.",
	"item-restored":      "Item restored from trash.",
	"item-purged":        "Item permanently deleted.",
	"status-updated":     "Status updated.",
	"maintenance-added":  "Added.",
	"project-added":      "Added.",
	"completion-logged":  "Logged.",
}

var flashErrorKeys = map[string]bool{
	"title-required":   true,
	"cadence-required": true,
	"cadence-invalid":  true,
	"item-purged":      true,
}

func resolveFlash(key string) string {
	return flashMessages[key]
}

// maintenanceView is a view-model for rendering maintenance items with computed fields.
type maintenanceView struct {
	MaintenanceItem
	IsOverdue    bool
	DueLabel     string
	DaysUntilDue int
}

// Handler serves the /house page combining maintenance and project services.
type Handler struct {
	maintenanceSvc *Service
	projectsSvc    *tracker.Service
	templates      map[string]*template.Template
	loc            *time.Location
	commentarySt   *commentary.Store
}

// NewHandler creates a house page handler.
func NewHandler(maintenanceSvc *Service, projectsSvc *tracker.Service, templates map[string]*template.Template, loc *time.Location) *Handler {
	return &Handler{
		maintenanceSvc: maintenanceSvc,
		projectsSvc:    projectsSvc,
		templates:      templates,
		loc:            loc,
	}
}

// SetCommentaryStore enables AI commentary for house items.
func (h *Handler) SetCommentaryStore(st *commentary.Store) {
	h.commentarySt = st
}

// HousePage renders GET /house with both maintenance and project sections.
func (h *Handler) HousePage(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(h.loc)

	maintItems, err := h.maintenanceSvc.List()
	if err != nil {
		httputil.ServerError(w, "listing items", err)
		return
	}

	var maintenance []maintenanceView
	for _, it := range maintItems {
		mv := maintenanceView{
			MaintenanceItem: it,
			IsOverdue:       it.IsOverdue(now, h.loc),
			DueLabel:        dueLabel(it, now, h.loc),
			DaysUntilDue:    it.DaysUntilDue(now, h.loc),
		}
		maintenance = append(maintenance, mv)
	}
	// Sort: overdue first (most overdue at top), then by days-until-due ascending.
	sortMaintenanceViews(maintenance, now, h.loc)

	projectItems, err := h.projectsSvc.List()
	if err != nil {
		httputil.ServerError(w, "listing items", err)
		return
	}

	var projects, doneProjects []tracker.Item
	for _, it := range projectItems {
		if it.Status == "done" || it.Status == "drop" {
			doneProjects = append(doneProjects, it)
		} else {
			projects = append(projects, it)
		}
	}

	deletedMaint := h.maintenanceSvc.ListDeleted()
	deletedProjects := h.projectsSvc.ListDeleted()

	data := auth.TemplateData(r)
	data["Title"] = "House"
	data["Maintenance"] = maintenance
	data["Projects"] = projects
	data["DoneProjects"] = doneProjects
	data["DeletedMaintenance"] = deletedMaint
	data["DeletedProjects"] = deletedProjects
	data["Now"] = now
	data["AllStatuses"] = []string{"todo", "active", "done", "drop"}

	if msg := r.URL.Query().Get("msg"); msg != "" {
		data["FlashMsg"] = resolveFlash(msg)
		data["FlashError"] = flashErrorKeys[msg]
	}

	h.templates["house.html"].ExecuteTemplate(w, "layout.html", data)
}

// --- Maintenance handlers ---

// AddMaintenance handles POST /house/maintenance/add.
func (h *Handler) AddMaintenance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/house?msg=title-required", http.StatusSeeOther)
		return
	}

	cadence := strings.TrimSpace(r.FormValue("cadence"))
	if cadence == "" {
		http.Redirect(w, r, "/house?msg=cadence-required", http.StatusSeeOther)
		return
	}
	if _, _, err := ParseCadence(cadence); err != nil {
		http.Redirect(w, r, "/house?msg=cadence-invalid", http.StatusSeeOther)
		return
	}

	var tags []string
	for t := range strings.SplitSeq(r.FormValue("tags"), ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	item := &MaintenanceItem{
		Title:   title,
		Cadence: cadence,
		Tags:    tags,
	}

	if err := h.maintenanceSvc.Add(item); err != nil {
		httputil.ServerError(w, "adding maintenance item", err)
		return
	}

	http.Redirect(w, r, "/house?msg=maintenance-added", http.StatusSeeOther)
}

// LogDone handles POST /house/maintenance/{slug}/log.
func (h *Handler) LogDone(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	note := strings.TrimSpace(r.FormValue("note"))
	if err := h.maintenanceSvc.LogCompletion(slug, note); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/house?msg=completion-logged#"+slug, http.StatusSeeOther)
}

// EditMaintenance handles POST /house/maintenance/{slug}/edit.
func (h *Handler) EditMaintenance(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	var tags []string
	for t := range strings.SplitSeq(r.FormValue("tags"), ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	if err := h.maintenanceSvc.UpdateEdit(slug, title, notes, tags); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/house?msg=item-edited", http.StatusSeeOther)
}

// DeleteMaintenance handles POST /house/maintenance/{slug}/delete.
func (h *Handler) DeleteMaintenance(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.maintenanceSvc.Delete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-deleted", http.StatusSeeOther)
}

// RestoreMaintenance handles POST /house/maintenance/{slug}/restore.
func (h *Handler) RestoreMaintenance(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.maintenanceSvc.Restore(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-restored", http.StatusSeeOther)
}

// PurgeMaintenance handles POST /house/maintenance/{slug}/purge.
func (h *Handler) PurgeMaintenance(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.maintenanceSvc.PermanentDelete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-purged", http.StatusSeeOther)
}

// --- Project handlers ---

// AddProject handles POST /house/projects/add.
func (h *Handler) AddProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/house?msg=title-required", http.StatusSeeOther)
		return
	}

	status := tracker.SanitiseStatus(r.FormValue("status"))
	if status == "" {
		status = "todo"
	}

	var budget float64
	if b := r.FormValue("budget"); b != "" {
		budget, _ = strconv.ParseFloat(b, 64)
		budget = tracker.SanitiseBudget(budget)
	}

	var tags []string
	for t := range strings.SplitSeq(r.FormValue("tags"), ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	body := strings.TrimSpace(r.FormValue("body"))

	item := tracker.Item{
		Title:  title,
		Type:   tracker.TaskType,
		Status: status,
		Budget: budget,
		Tags:   tags,
		Body:   body,
	}

	if err := h.projectsSvc.AddItem(item); err != nil {
		httputil.ServerError(w, "adding project", err)
		return
	}

	http.Redirect(w, r, "/house?msg=project-added", http.StatusSeeOther)
}

// CompleteProject handles POST /house/projects/{slug}/complete.
func (h *Handler) CompleteProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.projectsSvc.Complete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house", http.StatusSeeOther)
}

// UncompleteProject handles POST /house/projects/{slug}/uncomplete.
func (h *Handler) UncompleteProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.projectsSvc.Uncomplete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house", http.StatusSeeOther)
}

// EditProject handles POST /house/projects/{slug}/edit.
func (h *Handler) EditProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	var tags []string
	for t := range strings.SplitSeq(r.FormValue("tags"), ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	if err := h.projectsSvc.UpdateEdit(slug, title, body, tags, nil); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-edited", http.StatusSeeOther)
}

// UpdateStatus handles POST /house/projects/{slug}/status.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	status := tracker.SanitiseStatus(r.FormValue("status"))
	if err := h.projectsSvc.UpdateStatus(slug, status); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=status-updated", http.StatusSeeOther)
}

// DeleteProject handles POST /house/projects/{slug}/delete.
func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.projectsSvc.Delete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-deleted", http.StatusSeeOther)
}

// RestoreProject handles POST /house/projects/{slug}/restore.
func (h *Handler) RestoreProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.projectsSvc.Restore(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-restored", http.StatusSeeOther)
}

// PurgeProject handles POST /house/projects/{slug}/purge.
func (h *Handler) PurgeProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.projectsSvc.PermanentDelete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/house?msg=item-purged", http.StatusSeeOther)
}

// --- Helpers ---

func dueLabel(it MaintenanceItem, now time.Time, loc *time.Location) string {
	due := it.NextDue(loc)
	if due.IsZero() {
		return "never done"
	}
	days := it.DaysUntilDue(now, loc)
	switch {
	case days < 0:
		return fmt.Sprintf("%d days overdue", -days)
	case days == 0:
		return "due today"
	case days == 1:
		return "due tomorrow"
	case days < 7:
		return fmt.Sprintf("due in %d days", days)
	case days < 14:
		return "due in 1 week"
	default:
		return fmt.Sprintf("due in %d weeks", days/7)
	}
}

func sortMaintenanceViews(views []maintenanceView, now time.Time, loc *time.Location) {
	for i := 1; i < len(views); i++ {
		for j := i; j > 0; j-- {
			a := views[j].MaintenanceItem.DaysUntilDue(now, loc)
			b := views[j-1].MaintenanceItem.DaysUntilDue(now, loc)
			if a < b {
				views[j], views[j-1] = views[j-1], views[j]
			} else {
				break
			}
		}
	}
}
