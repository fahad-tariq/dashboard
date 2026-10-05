package home

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/tracker"
)

// Lists are the services the core reads for one request: the three tracker
// lists the planner, calendar and plan API work on, plus ideas for the
// digest and the tag summary. Everything else on the homepage comes from
// module widgets.
type Lists struct {
	Personal      *tracker.Service
	Family        *tracker.Service
	HouseProjects *tracker.Service
	Ideas         *ideas.Service
}

// Resolver returns the lists for the request's user.
type Resolver func(r *http.Request) Lists

// Widgets returns the module widgets shown below the plan section.
type Widgets func(ctx context.Context, userID int64, now time.Time) []module.WidgetData

type Handler struct {
	resolve   Resolver
	widgets   Widgets
	templates map[string]*template.Template
	loc       *time.Location
}

func NewHandler(resolve Resolver, templates map[string]*template.Template, loc *time.Location) *Handler {
	return &Handler{resolve: resolve, templates: templates, loc: loc}
}

// SetWidgets adds module widgets to the homepage.
func (h *Handler) SetWidgets(w Widgets) {
	h.widgets = w
}

func (h *Handler) HomePage(w http.ResponseWriter, r *http.Request) {
	h.renderHomePage(w, r, h.resolve(r))
}

// Greeting returns a time-of-day greeting, optionally personalised with the
// user's name and contextual suffixes at natural rest points.
// Greetings rotate by day-of-year for variety without randomness.
func Greeting(now time.Time, name string, streakDays int, planAllDone bool) string {
	hour := now.Hour()
	doy := now.YearDay()

	var base string
	switch {
	case hour >= 5 && hour <= 11:
		pool := []string{"Good morning", "Morning"}
		base = pool[doy%len(pool)]
	case hour >= 12 && hour <= 17:
		pool := []string{"Good afternoon", "Afternoon"}
		base = pool[doy%len(pool)]
	default:
		pool := []string{"Good evening", "Evening"}
		base = pool[doy%len(pool)]
	}

	if name != "" {
		base += ", " + name
	}

	// Contextual suffixes -- rare triggers only.
	switch {
	case isStreakMilestone(streakDays) && hour >= 5 && hour <= 11:
		return fmt.Sprintf("%s. %d-day streak.", base, streakDays)
	case planAllDone && hour >= 12:
		return base + ". All clear for today."
	default:
		return base
	}
}

func isStreakMilestone(days int) bool {
	return days == 7 || days == 14 || days == 30 || days == 60 || days == 90 || days == 180 || days == 365
}

var defaultPlanPrompts = []string{
	"Anything for today?",         // Sunday
	"What needs doing?",           // Monday
	"What matters today?",         // Tuesday
	"Three things?",               // Wednesday
	"What would make today good?", // Thursday
	"Last stretch of the week",    // Friday
	"Anything for today?",         // Saturday
}

// PlanPrompt returns a context-aware prompt for the empty plan state.
func PlanPrompt(now time.Time, openTaskCount int, streakDays int) string {
	weekday := now.Weekday()

	switch {
	case weekday == time.Friday && openTaskCount > 0 && openTaskCount <= 3:
		return "Nearly there. Anything else?"
	case isStreakMilestone(streakDays):
		return fmt.Sprintf("Day %d. What's on?", streakDays)
	case openTaskCount == 0:
		return "All caught up. Anything new?"
	default:
		return defaultPlanPrompts[weekday]
	}
}

// planSection is one list's share of today's plan.
type planSection struct {
	planned   []tracker.Item // planned for today, then carried-over items
	carried   int
	unplanned []tracker.Item // open tasks for the picker
}

// buildPlanSection merges carried-over items into the plan and collects the
// open tasks that are neither planned nor carried over.
func buildPlanSection(svc *tracker.Service, items []tracker.Item, today string) planSection {
	planned := svc.ListPlanned(today)
	carried := svc.ListOverdue(today)
	exclude := make(map[string]bool, len(planned)+len(carried))
	for _, it := range planned {
		exclude[it.Slug] = true
	}
	for _, it := range carried {
		exclude[it.Slug] = true
	}
	var unplanned []tracker.Item
	for _, it := range items {
		if it.Type == tracker.TaskType && !it.Done && !exclude[it.Slug] {
			unplanned = append(unplanned, it)
		}
	}
	planned = append(planned, carried...)
	sortPlanItems(planned)
	sortByPriority(unplanned)
	return planSection{planned: planned, carried: len(carried), unplanned: unplanned}
}

func countDone(items []tracker.Item) int {
	n := 0
	for _, it := range items {
		if it.Done {
			n++
		}
	}
	return n
}

// tagInfos gathers tags from tasks, goals and unconverted ideas.
func tagInfos(personal, family []tracker.Item, allIdeas []ideas.Idea) []insights.TagInfo {
	var infos []insights.TagInfo
	for _, it := range slices.Concat(personal, family) {
		infos = append(infos, insights.TagInfo{Tags: it.Tags, Type: string(it.Type), Done: it.Done})
	}
	for _, idea := range allIdeas {
		if idea.Status != "converted" {
			infos = append(infos, insights.TagInfo{Tags: idea.Tags, Type: "idea", Done: false})
		}
	}
	return infos
}

func listOrLog(name string, list func() ([]tracker.Item, error)) []tracker.Item {
	items, err := list()
	if err != nil {
		slog.Error("homepage list", "list", name, "error", err)
	}
	return items
}

func (h *Handler) renderHomePage(w http.ResponseWriter, r *http.Request, l Lists) {
	personalItems := listOrLog("personal", l.Personal.List)
	familyItems := listOrLog("family", l.Family.List)
	houseItems := listOrLog("house", l.HouseProjects.List)
	allIdeas, err := l.Ideas.List()
	if err != nil {
		slog.Error("homepage ideas list", "error", err)
	}

	now := time.Now().In(h.loc)
	today := now.Format("2006-01-02")

	completedItems := append(toCompletedItems(personalItems), toCompletedItems(familyItems)...)
	streakDays, totalCompleted := insights.Streak(completedItems, now)

	personal := buildPlanSection(l.Personal, personalItems, today)
	family := buildPlanSection(l.Family, familyItems, today)
	houseSec := buildPlanSection(l.HouseProjects, houseItems, today)

	planTotal := len(personal.planned) + len(family.planned) + len(houseSec.planned)
	planDone := countDone(personal.planned) + countDone(family.planned) + countDone(houseSec.planned)
	planAllDone := planTotal > 0 && planDone == planTotal
	unplanned := len(personal.unplanned) + len(family.unplanned) + len(houseSec.unplanned)
	var widgets []module.WidgetData
	if h.widgets != nil {
		widgets = h.widgets(r.Context(), auth.UserID(r.Context()), now)
	}

	data := auth.TemplateData(r)
	userName, _ := data["UserName"].(string)
	data["Title"] = "Home"
	data["Greeting"] = Greeting(now, userName, streakDays, planAllDone)
	data["DateLabel"] = formatDateLabel(now)
	data["Today"] = today
	data["PersonalPlanned"] = personal.planned
	data["FamilyPlanned"] = family.planned
	data["HousePlanned"] = houseSec.planned
	data["PersonalCarriedCount"] = personal.carried
	data["FamilyCarriedCount"] = family.carried
	data["HouseCarriedCount"] = houseSec.carried
	data["CarriedOverCount"] = personal.carried + family.carried + houseSec.carried
	data["UnplannedPersonal"] = personal.unplanned
	data["UnplannedFamily"] = family.unplanned
	data["UnplannedHouse"] = houseSec.unplanned
	data["PlanDoneCount"] = planDone
	data["PlanTotalCount"] = planTotal
	data["PlanAllDone"] = planAllDone
	data["PlanPrompt"] = PlanPrompt(now, countOpenTasks(personalItems)+countOpenTasks(familyItems), streakDays)
	// With nothing to plan and no widget to show, the page shows a getting
	// started message instead.
	data["Empty"] = planTotal == 0 && unplanned == 0 && len(widgets) == 0
	data["Widgets"] = widgets
	data["InsightLine"] = insights.WeeklyVelocity(completedItems, now)
	data["StreakDays"] = streakDays
	data["TotalCompleted"] = totalCompleted
	data["MilestoneBadge"] = insights.MilestoneBadge(totalCompleted)
	data["TagSummaries"] = insights.TopN(insights.TagAggregation(tagInfos(personalItems, familyItems, allIdeas)), 5)

	if msgKey := r.URL.Query().Get("msg"); msgKey != "" {
		if flashMsg := resolvePlanFlash(msgKey, now); flashMsg != "" {
			data["FlashMsg"] = flashMsg
		}
	}

	if err := h.templates["homepage.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering homepage", "error", err)
	}
}

// formatDateLabel returns a human-readable date like "Thursday, 19 March".
func formatDateLabel(t time.Time) string {
	return t.Format("Monday, 2 January")
}

func sortByPriority(items []tracker.Item) {
	slices.SortFunc(items, func(a, b tracker.Item) int {
		return tracker.PriorityWeight[a.Priority] - tracker.PriorityWeight[b.Priority]
	})
}

// sortPlanItems sorts planned items: explicit PlanOrder first (ascending),
// then unordered items by priority weight.
func sortPlanItems(items []tracker.Item) {
	slices.SortStableFunc(items, func(a, b tracker.Item) int {
		aHas := a.PlanOrder > 0
		bHas := b.PlanOrder > 0
		switch {
		case aHas && bHas:
			return a.PlanOrder - b.PlanOrder
		case aHas:
			return -1
		case bHas:
			return 1
		default:
			return tracker.PriorityWeight[a.Priority] - tracker.PriorityWeight[b.Priority]
		}
	})
}

func toCompletedItems(items []tracker.Item) []insights.CompletedItem {
	result := make([]insights.CompletedItem, len(items))
	for i, it := range items {
		result[i] = insights.CompletedItem{Completed: it.Completed, Done: it.Done}
	}
	return result
}

func countOpenTasks(items []tracker.Item) int {
	count := 0
	for _, it := range items {
		if it.Type == tracker.TaskType && !it.Done {
			count++
		}
	}
	return count
}

var planFlashMessages = map[string]string{
	"plan-cleared":    "Removed from plan.",
	"plan-bulk-set":   "Tasks added to today's plan.",
	"carried-cleared": "Carried-over tasks dropped.",
	"plan-reordered":  "Plan order updated.",
}

var rotatingPlanFlash = map[string][]string{
	"plan-set":       {"Planned.", "On today's list.", "Locked in."},
	"plan-completed": {"Done.", "Nice one.", "Sorted.", "Ticked off."},
}

func resolvePlanFlash(key string, now time.Time) string {
	if variants, ok := rotatingPlanFlash[key]; ok {
		return httputil.RotatingFlash(key, variants, now)
	}
	return planFlashMessages[key]
}

// SetPlanned handles POST /plan/set -- adds a task to the daily plan.
func (h *Handler) SetPlanned(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	slug := strings.TrimSpace(r.FormValue("slug"))
	list := strings.TrimSpace(r.FormValue("list"))
	date := strings.TrimSpace(r.FormValue("date"))
	if slug == "" || list == "" {
		http.Error(w, "Missing slug or list", http.StatusBadRequest)
		return
	}
	if date == "" {
		date = time.Now().In(h.loc).Format("2006-01-02")
	}

	svc := h.serviceForList(r, list)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}

	if err := svc.SetPlanned(slug, date); err != nil {
		http.Error(w, "Item not found", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/?msg=plan-set", http.StatusSeeOther)
}

// ClearPlanned handles POST /plan/clear -- removes a task from the plan.
func (h *Handler) ClearPlanned(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	slug := strings.TrimSpace(r.FormValue("slug"))
	list := strings.TrimSpace(r.FormValue("list"))
	if slug == "" || list == "" {
		http.Error(w, "Missing slug or list", http.StatusBadRequest)
		return
	}

	svc := h.serviceForList(r, list)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}

	if err := svc.ClearPlanned(slug); err != nil {
		http.Error(w, "Item not found", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/?msg=plan-cleared", http.StatusSeeOther)
}

// CompletePlanned handles POST /plan/{slug}/complete -- completes a task from the plan view.
func (h *Handler) CompletePlanned(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	slug := chi.URLParam(r, "slug")
	list := strings.TrimSpace(r.FormValue("list"))
	if slug == "" || list == "" {
		http.Error(w, "Missing slug or list", http.StatusBadRequest)
		return
	}

	svc := h.serviceForList(r, list)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}

	if err := svc.Complete(slug); err != nil {
		http.Error(w, "Item not found", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/?msg=plan-completed", http.StatusSeeOther)
}

// BulkSetPlanned handles POST /plan/bulk/set -- adds multiple tasks to the plan.
func (h *Handler) BulkSetPlanned(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	list := strings.TrimSpace(r.FormValue("list"))
	date := strings.TrimSpace(r.FormValue("date"))
	if len(slugs) == 0 || list == "" {
		http.Error(w, "No items selected", http.StatusBadRequest)
		return
	}
	if date == "" {
		date = time.Now().In(h.loc).Format("2006-01-02")
	}

	svc := h.serviceForList(r, list)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}

	if err := svc.BulkSetPlanned(slugs, date); err != nil {
		http.Error(w, "Failed to update items", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/?msg=plan-bulk-set", http.StatusSeeOther)
}

// ReorderPlanned handles POST /plan/reorder -- sets manual sort order for plan items.
func (h *Handler) ReorderPlanned(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	slugs := httputil.ParseCSV(r.FormValue("slugs"))
	list := strings.TrimSpace(r.FormValue("list"))
	if len(slugs) == 0 || list == "" {
		http.Error(w, "Missing slugs or list", http.StatusBadRequest)
		return
	}

	svc := h.serviceForList(r, list)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}

	if err := svc.ReorderPlanned(slugs); err != nil {
		http.Error(w, "Failed to reorder", http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" || r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/?msg=plan-reordered", http.StatusSeeOther)
}

// ClearCarriedOver handles POST /plan/bulk/clear-carried -- drops all overdue planned items.
func (h *Handler) ClearCarriedOver(w http.ResponseWriter, r *http.Request) {
	h.clearAllOverdue(h.resolve(r))
	http.Redirect(w, r, "/?msg=carried-cleared", http.StatusSeeOther)
}

func (h *Handler) clearAllOverdue(l Lists) {
	today := time.Now().In(h.loc).Format("2006-01-02")
	for _, svc := range []*tracker.Service{l.Personal, l.Family, l.HouseProjects} {
		for _, it := range svc.ListOverdue(today) {
			_ = svc.ClearPlanned(it.Slug)
		}
	}
}

// serviceForList returns the request user's tracker service for list, or nil.
func (h *Handler) serviceForList(r *http.Request, list string) *tracker.Service {
	return listService(h.resolve(r), list)
}

func listService(l Lists, list string) *tracker.Service {
	switch list {
	case "todos", "personal":
		return l.Personal
	case "family":
		return l.Family
	case "house":
		return l.HouseProjects
	}
	return nil
}

// APIListPlan handles GET /api/v1/plan?date=YYYY-MM-DD.
func (h *Handler) APIListPlan(w http.ResponseWriter, r *http.Request) {
	l := h.resolve(r)
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().In(h.loc).Format("2006-01-02")
	}

	overdue := slices.Concat(l.Personal.ListOverdue(date), l.Family.ListOverdue(date), l.HouseProjects.ListOverdue(date))
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"date":     date,
		"personal": planItemsToAPI(l.Personal.ListPlanned(date), "personal"),
		"family":   planItemsToAPI(l.Family.ListPlanned(date), "family"),
		"house":    planItemsToAPI(l.HouseProjects.ListPlanned(date), "house"),
		"overdue":  planItemsToAPI(overdue, ""),
	})
}

// APISetPlan handles PUT /api/v1/plan/{slug}.
func (h *Handler) APISetPlan(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var body struct {
		Date string `json:"date"`
		List string `json:"list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Date == "" {
		body.Date = time.Now().In(h.loc).Format("2006-01-02")
	}

	svc := h.serviceForList(r, body.List)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}
	if err := svc.SetPlanned(slug, body.Date); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// APIClearPlan handles DELETE /api/v1/plan/{slug}.
func (h *Handler) APIClearPlan(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var body struct {
		List string `json:"list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	svc := h.serviceForList(r, body.List)
	if svc == nil {
		http.Error(w, "Invalid list", http.StatusBadRequest)
		return
	}
	if err := svc.ClearPlanned(slug); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func planItemsToAPI(items []tracker.Item, list string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		m := map[string]any{
			"slug":     it.Slug,
			"title":    it.Title,
			"priority": it.Priority,
			"done":     it.Done,
			"planned":  it.Planned,
			"tags":     it.Tags,
		}
		if list != "" {
			m["list"] = list
		}
		out = append(out, m)
	}
	return out
}
