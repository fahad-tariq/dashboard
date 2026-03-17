package tracker

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/ideas"
)

var PriorityWeight = map[string]int{"high": 0, "medium": 1, "low": 2, "": 3}
var validPriorities = map[string]bool{"": true, "high": true, "medium": true, "low": true}

var flashMessages = map[string]string{
	"title-required": "Title is required.",
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
}

func NewHandler(svc, otherSvc *Service, templates map[string]*template.Template, listName string) *Handler {
	return &Handler{
		resolve: func(r *http.Request) (*Service, *Service) {
			return svc, otherSvc
		},
		templates: templates,
		listName:  listName,
	}
}

// NewHandlerWithResolver creates a handler that resolves services per-request.
func NewHandlerWithResolver(resolver ServiceResolver, templates map[string]*template.Template, listName string) *Handler {
	return &Handler{
		resolve:   resolver,
		templates: templates,
		listName:  listName,
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

func (h *Handler) TrackerPage(w http.ResponseWriter, r *http.Request) {
	svc, _ := h.resolve(r)
	items, err := svc.List()
	if err != nil {
		slog.Error("listing tracker items", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
	data["Categories"] = allTags
	data["Priorities"] = priorities
	if flashMsg := flashMessages[r.URL.Query().Get("msg")]; flashMsg != "" {
		data["FlashMsg"] = flashMsg
		data["FlashError"] = true
	}
	if userName := data["UserName"]; h.listName == "todos" && userName != "" {
		data["Subtitle"] = userName.(string) + "'s list"
	}

	if err := h.templates["tracker.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering tracker", "error", err)
	}
}

func (h *Handler) GoalsPage(w http.ResponseWriter, r *http.Request) {
	svc, _ := h.resolve(r)
	items, err := svc.List()
	if err != nil {
		slog.Error("listing tracker items", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var goals []Item
	for _, it := range items {
		if it.Type == GoalType {
			goals = append(goals, it)
		}
	}
	sortItems(goals)

	allTags, priorities := collectFilters(goals)

	data := auth.TemplateData(r)
	data["Title"] = "Goals"
	data["Goals"] = goals
	data["Categories"] = allTags
	data["Priorities"] = priorities
	if flashMsg := flashMessages[r.URL.Query().Get("msg")]; flashMsg != "" {
		data["FlashMsg"] = flashMsg
		data["FlashError"] = true
	}
	if userName := data["UserName"]; userName != "" {
		data["Subtitle"] = userName.(string) + "'s goals"
	}

	if err := h.templates["goals.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering goals", "error", err)
	}
}

func (h *Handler) QuickAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
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
		Tags:     ideas.ParseCSV(r.FormValue("tags")),
		Images:   ideas.ParseCSV(r.FormValue("images")),
	}

	svc, _ := h.resolve(r)
	if err := svc.AddItem(item); err != nil {
		slog.Error("adding task", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/"+h.listName, http.StatusSeeOther)
}

func (h *Handler) AddGoal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
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
		Body:     strings.TrimSpace(r.FormValue("body")),
		Tags:     ideas.ParseCSV(r.FormValue("tags")),
		Images:   ideas.ParseCSV(r.FormValue("images")),
	}

	svc, _ := h.resolve(r)
	if err := svc.AddItem(item); err != nil {
		slog.Error("adding goal", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/goals", http.StatusSeeOther)
}

func (h *Handler) redirectBack(w http.ResponseWriter, r *http.Request, anchor string) {
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
	if anchor != "" {
		dest += "#" + anchor
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (h *Handler) UpdateNotes(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	svc, _ := h.resolve(r)
	if err := svc.UpdateNotes(slug, body); err != nil {
		slog.Error("updating notes", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	h.redirectBack(w, r, slug)
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Complete(slug); err != nil {
		slog.Error("completing tracker item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "")
}

func (h *Handler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Uncomplete(slug); err != nil {
		slog.Error("uncompleting tracker item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "")
}

func (h *Handler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
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
			slog.Error("setting progress", "slug", slug, "error", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
	} else {
		if err := svc.UpdateProgress(slug, val); err != nil {
			slog.Error("updating progress", "slug", slug, "error", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
	}

	h.redirectBack(w, r, slug)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	svc, _ := h.resolve(r)
	if err := svc.Delete(slug); err != nil {
		slog.Error("deleting tracker item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, "")
}

func (h *Handler) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	priority := sanitisePriority(r.FormValue("priority"))
	svc, _ := h.resolve(r)
	if err := svc.UpdatePriority(slug, priority); err != nil {
		slog.Error("updating priority", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, slug)
}

func (h *Handler) UpdateTags(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	tags := ideas.ParseCSV(r.FormValue("tags"))
	svc, _ := h.resolve(r)
	if err := svc.UpdateTags(slug, tags); err != nil {
		slog.Error("updating tags", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	h.redirectBack(w, r, slug)
}

func (h *Handler) UpdateEdit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	tags := ideas.ParseCSV(r.FormValue("tags"))
	images := ideas.ParseCSV(r.FormValue("images"))

	svc, _ := h.resolve(r)
	if err := svc.UpdateEdit(slug, body, tags, images); err != nil {
		slog.Error("updating item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	h.redirectBack(w, r, slug)
}

func (h *Handler) MoveToList(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	svc, otherSvc := h.resolve(r)
	item, err := svc.Get(slug)
	if err != nil {
		slog.Error("getting item for move", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	movedItem := *item

	if err := svc.Delete(slug); err != nil {
		slog.Error("deleting item from source list", "slug", slug, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := otherSvc.Get(movedItem.Slug); err == nil {
		movedItem.Slug = movedItem.Slug + "-" + fmt.Sprintf("%d", time.Now().Unix())
	}

	if err := otherSvc.AddItem(movedItem); err != nil {
		slog.Warn("item deleted from source but failed to add to target, manual recovery may be needed",
			"slug", slug, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/"+h.listName, http.StatusSeeOther)
}
