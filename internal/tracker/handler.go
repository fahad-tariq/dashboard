package tracker

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/httputil"
)

var PriorityWeight = map[string]int{"high": 0, "medium": 1, "low": 2, "": 3}
var validPriorities = map[string]bool{"": true, "high": true, "medium": true, "low": true}

var flashMessages = map[string]string{
	"title-required":   "A title is required.",
	"task-uncompleted": "Task reopened.",
	"notes-updated":    "Notes saved.",
	"priority-updated": "Priority updated.",
	"tags-updated":     "Tags updated.",
	"item-updated":     "Changes saved.",
	"item-deleted":     "Item moved to trash.",
	"item-moved":       "Moved to the other list.",
	"item-restored":    "Item restored from trash.",
	"item-purged":      "Item permanently deleted.",
	"bulk-deleted":     "Items moved to trash.",
	"bulk-priority":    "Priority updated for selected items.",
	"bulk-tagged":      "Tag added to selected items.",
	"bulk-planned":     "Tasks added to today's plan.",
}

var rotatingFlashMessages = map[string][]string{
	"task-added":     {"Added.", "On the list.", "Captured."},
	"goal-added":     {"Added.", "Tracked."},
	"task-completed": {"Done.", "Nice one.", "Sorted.", "Ticked off."},
	"bulk-completed": {"Done.", "All sorted.", "Ticked off."},
	"plan-set":       {"Planned.", "On today's list.", "Locked in."},
}

func resolveFlash(key string, now time.Time) string {
	if variants, ok := rotatingFlashMessages[key]; ok {
		return httputil.RotatingFlash(key, variants, now)
	}
	return flashMessages[key]
}

var flashErrorKeys = map[string]bool{
	"title-required": true,
	"item-purged":    true,
	"bulk-deleted":   true,
}

func sanitisePriority(p string) string {
	if validPriorities[p] {
		return p
	}
	return ""
}

// ServiceResolver returns the (svc, otherSvc) pair for a request.
// For personal handlers, svc is the user's personal service and otherSvc is family.
// For family handlers, svc is family and otherSvc is the user's personal service.
type ServiceResolver func(r *http.Request) (svc *Service, otherSvc *Service)

type Handler struct {
	resolve   ServiceResolver
	templates map[string]*template.Template
	listName  string
	loc       *time.Location
}

func NewHandler(svc, otherSvc *Service, templates map[string]*template.Template, listName string, loc *time.Location) *Handler {
	return &Handler{
		resolve: func(r *http.Request) (*Service, *Service) {
			return svc, otherSvc
		},
		templates: templates,
		listName:  listName,
		loc:       loc,
	}
}

// NewHandlerWithResolver creates a handler that resolves services per-request.
func NewHandlerWithResolver(resolver ServiceResolver, templates map[string]*template.Template, listName string, loc *time.Location) *Handler {
	return &Handler{
		resolve:   resolver,
		templates: templates,
		listName:  listName,
		loc:       loc,
	}
}

func (h *Handler) otherListName() string {
	if h.listName == "todos" {
		return "family"
	}
	return "todos"
}

func sortItems(s []Item) {
	slices.SortFunc(s, func(a, b Item) int {
		pa, pb := PriorityWeight[a.Priority], PriorityWeight[b.Priority]
		if pa != pb {
			return pa - pb
		}
		if a.Added != b.Added {
			if a.Added == "" {
				return 1
			}
			if b.Added == "" {
				return -1
			}
			if a.Added > b.Added {
				return -1
			}
			return 1
		}
		return 0
	})
}

func collectFilters(items []Item) (allTags, priorities []string) {
	tagSet := map[string]bool{}
	priSet := map[string]bool{}
	for _, it := range items {
		for _, t := range it.Tags {
			tagSet[strings.ToLower(t)] = true
		}
		if it.Priority != "" {
			priSet[it.Priority] = true
		}
	}
	for t := range tagSet {
		allTags = append(allTags, t)
	}
	slices.Sort(allTags)
	for _, p := range []string{"high", "medium", "low"} {
		if priSet[p] {
			priorities = append(priorities, p)
		}
	}
	return
}

// classifyTrackerError returns an appropriate HTTP error message for a service error.
func classifyTrackerError(err error) string {
	if httputil.IsNotFound(err) {
		return "Item not found"
	}
	return "Failed to update item"
}

func (h *Handler) TrackerPage(w http.ResponseWriter, r *http.Request) {
	svc, _ := h.resolve(r)
	items, err := svc.List()
	if err != nil {
		httputil.ServerError(w, "listing tracker items", err)
		return
	}

	var tasks, doneTasks []Item
	for _, it := range items {
		if it.Type != TaskType {
			continue
		}
		if it.Done {
			doneTasks = append(doneTasks, it)
		} else {
			tasks = append(tasks, it)
		}
	}
	sortItems(tasks)

	// Collect deleted tasks for the "Recently Deleted" section.
	var deletedTasks []Item
	for _, it := range svc.ListDeleted() {
		if it.Type == TaskType {
			deletedTasks = append(deletedTasks, it)
		}
	}

	allTags, priorities := collectFilters(tasks)

	var title string
	if h.listName == "todos" {
		title = "Todos"
	} else {
		title = strings.ToUpper(h.listName[:1]) + h.listName[1:] + " Tasks"
	}
	data := auth.TemplateData(r)
	data["Title"] = title
	data["ListName"] = h.listName
	data["OtherListName"] = h.otherListName()
	data["Tasks"] = tasks
	data["DoneTasks"] = doneTasks
	data["DeletedTasks"] = deletedTasks
	data["Categories"] = allTags
	data["Priorities"] = priorities
	if msgKey := r.URL.Query().Get("msg"); msgKey != "" {
		if flashMsg := resolveFlash(msgKey, time.Now().In(h.loc)); flashMsg != "" {
			data["FlashMsg"] = flashMsg
			data["FlashError"] = flashErrorKeys[msgKey]
		}
	}
	if userName, ok := data["UserName"].(string); ok && h.listName == "todos" && userName != "" {
		data["Subtitle"] = userName + "'s list"
	}

	if err := h.templates["tracker.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		httputil.ServerError(w, "rendering tracker", err)
	}
}

func (h *Handler) GoalsPage(w http.ResponseWriter, r *http.Request) {
	svc, _ := h.resolve(r)
	items, err := svc.List()
	if err != nil {
		httputil.ServerError(w, "listing tracker items", err)
		return
	}

	var goals []Item
	for _, it := range items {
		if it.Type == GoalType {
			goals = append(goals, it)
		}
	}
	sortItems(goals)

	// Collect deleted goals for the "Recently Deleted" section.
	var deletedGoals []Item
	for _, it := range svc.ListDeleted() {
		if it.Type == GoalType {
			deletedGoals = append(deletedGoals, it)
		}
	}

	allTags, priorities := collectFilters(goals)

	data := auth.TemplateData(r)
	data["Title"] = "Goals"
	data["Goals"] = goals
	data["DeletedGoals"] = deletedGoals
	data["Categories"] = allTags
	data["Priorities"] = priorities
	if msgKey := r.URL.Query().Get("msg"); msgKey != "" {
		if flashMsg := resolveFlash(msgKey, time.Now().In(h.loc)); flashMsg != "" {
			data["FlashMsg"] = flashMsg
			data["FlashError"] = flashErrorKeys[msgKey]
		}
	}
	if userName, ok := data["UserName"].(string); ok && userName != "" {
		data["Subtitle"] = userName + "'s goals"
	}

	if err := h.templates["goals.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		httputil.ServerError(w, "rendering goals", err)
	}
}

func (h *Handler) QuickAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/"+h.listName+"?msg=title-required", http.StatusSeeOther)
		return
	}

	item := Item{
		Title:    title,
		Type:     TaskType,
		Priority: sanitisePriority(r.FormValue("priority")),
		Body:     strings.TrimSpace(r.FormValue("body")),
		Tags:     httputil.ParseCSV(r.FormValue("tags")),
		Images:   httputil.ReconstructImages(r),
	}

	svc, _ := h.resolve(r)
	if err := svc.AddItem(item); err != nil {
		httputil.ServerError(w, "adding task", err)
		return
	}

	http.Redirect(w, r, "/"+h.listName+"?msg=task-added", http.StatusSeeOther)
}

func (h *Handler) AddGoal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/goals?msg=title-required", http.StatusSeeOther)
		return
	}

	current, _ := strconv.ParseFloat(r.FormValue("current"), 64)
	target, _ := strconv.ParseFloat(r.FormValue("target"), 64)

	item := Item{
		Title:    title,
		Type:     GoalType,
		Priority: sanitisePriority(r.FormValue("priority")),
		Current:  current,
		Target:   target,
		Unit:     strings.TrimSpace(r.FormValue("unit")),
		Deadline: strings.TrimSpace(r.FormValue("deadline")),
		Body:     strings.TrimSpace(r.FormValue("body")),
		Tags:     httputil.ParseCSV(r.FormValue("tags")),
		Images:   httputil.ReconstructImages(r),
	}

	svc, _ := h.resolve(r)
	if err := svc.AddItem(item); err != nil {
		httputil.ServerError(w, "adding goal", err)
		return
	}

	http.Redirect(w, r, "/goals?msg=goal-added", http.StatusSeeOther)
}

func (h *Handler) redirectBack(w http.ResponseWriter, r *http.Request, anchor string, msg ...string) {
	dest := r.Header.Get("Referer")

	if dest != "" {
		if u, err := url.Parse(dest); err == nil {
			dest = u.Path
		} else {
			dest = "/" + h.listName
		}
	}
	if dest == "" || !strings.HasPrefix(dest, "/") {
		dest = "/" + h.listName
	}
	if len(msg) > 0 && msg[0] != "" {
		dest += "?msg=" + msg[0]
	}
	if anchor != "" {
		dest += "#" + anchor
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (h *Handler) UpdateNotes(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	svc, _ := h.resolve(r)
	if err := svc.UpdateNotes(slug, body); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}

	h.redirectBack(w, r, slug, "notes-updated")
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Complete(slug); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "task-completed")
}

func (h *Handler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Uncomplete(slug); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "task-uncompleted")
}

func (h *Handler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	val, err := strconv.ParseFloat(r.FormValue("delta"), 64)
	if err != nil {
		http.Error(w, "Invalid value", http.StatusBadRequest)
		return
	}

	svc, _ := h.resolve(r)
	if r.FormValue("set") != "" {
		if err := svc.SetProgress(slug, val); err != nil {
			http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
			return
		}
	} else {
		if err := svc.UpdateProgress(slug, val); err != nil {
			http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
			return
		}
	}

	h.redirectBack(w, r, slug)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Delete(slug); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "item-deleted")
}

func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Restore(slug); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "item-restored")
}

func (h *Handler) Purge(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.PermanentDelete(slug); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "item-purged")
}

func (h *Handler) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	priority := sanitisePriority(r.FormValue("priority"))
	svc, _ := h.resolve(r)
	if err := svc.UpdatePriority(slug, priority); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, slug, "priority-updated")
}

func (h *Handler) UpdateTags(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	tags := httputil.ParseCSV(r.FormValue("tags"))
	svc, _ := h.resolve(r)
	if err := svc.UpdateTags(slug, tags); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, slug, "tags-updated")
}

func (h *Handler) UpdateEdit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tags := httputil.ParseCSV(r.FormValue("tags"))
	images := httputil.ReconstructImages(r)

	svc, _ := h.resolve(r)
	if err := svc.UpdateEdit(slug, title, body, tags, images); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}

	h.redirectBack(w, r, slug, "item-updated")
}

func (h *Handler) BulkComplete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	if len(slugs) == 0 {
		http.Error(w, "No items selected", http.StatusBadRequest)
		return
	}
	svc, _ := h.resolve(r)
	if err := svc.BulkComplete(slugs); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "bulk-completed")
}

func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	if len(slugs) == 0 {
		http.Error(w, "No items selected", http.StatusBadRequest)
		return
	}
	svc, _ := h.resolve(r)
	if err := svc.BulkDelete(slugs); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "bulk-deleted")
}

func (h *Handler) BulkPriority(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	if len(slugs) == 0 {
		http.Error(w, "No items selected", http.StatusBadRequest)
		return
	}
	priority := sanitisePriority(r.FormValue("priority"))
	svc, _ := h.resolve(r)
	if err := svc.BulkUpdatePriority(slugs, priority); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "bulk-priority")
}

func (h *Handler) BulkAddTag(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	if len(slugs) == 0 {
		http.Error(w, "No items selected", http.StatusBadRequest)
		return
	}
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" {
		http.Error(w, "Tag is required", http.StatusBadRequest)
		return
	}
	svc, _ := h.resolve(r)
	if err := svc.BulkAddTag(slugs, tag); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "bulk-tagged")
}

func (h *Handler) PlanForToday(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	today := time.Now().In(h.loc).Format("2006-01-02")
	svc, _ := h.resolve(r)
	if err := svc.SetPlanned(slug, today); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "plan-set")
}

func (h *Handler) BulkPlanForToday(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	if len(slugs) == 0 {
		http.Error(w, "No items selected", http.StatusBadRequest)
		return
	}
	today := time.Now().In(h.loc).Format("2006-01-02")
	svc, _ := h.resolve(r)
	if err := svc.BulkSetPlanned(slugs, today); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "", "bulk-planned")
}

func (h *Handler) AddSubStep(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		h.redirectBack(w, r, "item-"+slug)
		return
	}
	svc, _ := h.resolve(r)
	if err := svc.AddSubStep(slug, text); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "item-"+slug)
}

func (h *Handler) ToggleSubStep(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(r.FormValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}
	svc, _ := h.resolve(r)
	if err := svc.ToggleSubStep(slug, index); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "item-"+slug)
}

func (h *Handler) RemoveSubStep(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(r.FormValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}
	svc, _ := h.resolve(r)
	if err := svc.RemoveSubStep(slug, index); err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "item-"+slug)
}

func (h *Handler) PromoteSubStep(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(r.FormValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}
	svc, _ := h.resolve(r)
	item, err := svc.Get(slug)
	if err != nil {
		http.Error(w, classifyTrackerError(err), http.StatusBadRequest)
		return
	}
	steps := ParseSubSteps(item.Body)
	if index >= len(steps) {
		http.Error(w, "Sub-step not found", http.StatusBadRequest)
		return
	}
	text := steps[index].Text
	// Mark the step as done (leaves it as a record) rather than removing it.
	if !steps[index].Done {
		if err := svc.ToggleSubStep(slug, index); err != nil {
			httputil.ServerError(w, "marking sub-step done", err)
			return
		}
	}
	if err := svc.AddItem(Item{Title: text, Type: TaskType}); err != nil {
		httputil.ServerError(w, "promoting sub-step to task", err)
		return
	}
	h.redirectBack(w, r, "item-"+slug)
}

func (h *Handler) MoveToList(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	svc, otherSvc := h.resolve(r)
	item, err := svc.Get(slug)
	if err != nil {
		http.Error(w, "Item not found", http.StatusBadRequest)
		return
	}

	movedItem := *item

	if err := svc.PermanentDelete(slug); err != nil {
		httputil.ServerError(w, "deleting item from source list", err, "slug", slug)
		return
	}

	if _, err := otherSvc.Get(movedItem.Slug); err == nil {
		movedItem.Slug = movedItem.Slug + "-" + fmt.Sprintf("%d", time.Now().Unix())
	}

	if err := otherSvc.AddItem(movedItem); err != nil {
		httputil.ServerError(w, "item deleted from source but failed to add to target, manual recovery may be needed", err, "slug", slug)
		return
	}

	http.Redirect(w, r, "/"+h.listName+"?msg=item-moved", http.StatusSeeOther)
}
