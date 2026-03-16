package tracker

import (
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

var priorityWeight = map[string]int{"high": 0, "medium": 1, "low": 2, "": 3}
var validPriorities = map[string]bool{"": true, "high": true, "medium": true, "low": true}

func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func sanitisePriority(p string) string {
	if validPriorities[p] {
		return p
	}
	return ""
}

type Handler struct {
	svc       *Service
	templates map[string]*template.Template
}

func NewHandler(svc *Service, templates map[string]*template.Template) *Handler {
	return &Handler{svc: svc, templates: templates}
}

func sortItems(s []Item) {
	slices.SortFunc(s, func(a, b Item) int {
		pa, pb := priorityWeight[a.Priority], priorityWeight[b.Priority]
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
			tagSet[t] = true
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
	items, err := h.svc.List()
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

	data := map[string]any{
		"Title":      "Tasks",
		"Tasks":      tasks,
		"DoneTasks":  doneTasks,
		"Categories": allTags,
		"Priorities": priorities,
	}

	if err := h.templates["tracker.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering tracker", "error", err)
	}
}

func (h *Handler) GoalsPage(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List()
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

	data := map[string]any{
		"Title":      "Goals",
		"Goals":      goals,
		"Categories": allTags,
		"Priorities": priorities,
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
		http.Error(w, "Title required", http.StatusBadRequest)
		return
	}

	item := Item{
		Title:    title,
		Type:     TaskType,
		Priority: sanitisePriority(r.FormValue("priority")),
		Body:     strings.TrimSpace(r.FormValue("body")),
		Tags:     parseTags(r.FormValue("tags")),
	}

	if err := h.svc.AddItem(item); err != nil {
		slog.Error("adding task", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) AddGoal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "Title required", http.StatusBadRequest)
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
		Tags:     parseTags(r.FormValue("tags")),
	}

	if err := h.svc.AddItem(item); err != nil {
		slog.Error("adding goal", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/goals", http.StatusSeeOther)
}

func redirectBack(w http.ResponseWriter, r *http.Request, anchor string) {
	dest := r.Header.Get("Referer")

	if dest != "" {
		if u, err := url.Parse(dest); err == nil {
			dest = u.Path
		} else {
			dest = "/"
		}
	}
	if dest == "" || !strings.HasPrefix(dest, "/") {
		dest = "/"
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
	if err := h.svc.UpdateNotes(slug, body); err != nil {
		slog.Error("updating notes", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	redirectBack(w, r, slug)
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.svc.Complete(slug); err != nil {
		slog.Error("completing tracker item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	redirectBack(w, r, "")
}

func (h *Handler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.svc.Uncomplete(slug); err != nil {
		slog.Error("uncompleting tracker item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	redirectBack(w, r, "")
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

	if r.FormValue("set") != "" {
		if err := h.svc.SetProgress(slug, val); err != nil {
			slog.Error("setting progress", "slug", slug, "error", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
	} else {
		if err := h.svc.UpdateProgress(slug, val); err != nil {
			slog.Error("updating progress", "slug", slug, "error", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
	}

	redirectBack(w, r, slug)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.svc.Delete(slug); err != nil {
		slog.Error("deleting tracker item", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	redirectBack(w, r, "")
}

func (h *Handler) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	priority := sanitisePriority(r.FormValue("priority"))
	if err := h.svc.UpdatePriority(slug, priority); err != nil {
		slog.Error("updating priority", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	redirectBack(w, r, slug)
}

func (h *Handler) UpdateTags(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("tags"))
	var tags []string
	if raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	if err := h.svc.UpdateTags(slug, tags); err != nil {
		slog.Error("updating tags", "slug", slug, "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	redirectBack(w, r, slug)
}
