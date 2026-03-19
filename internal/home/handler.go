package home

import (
	"encoding/json"
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
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/tracker"
)

type Handler struct {
	registry  *services.Registry
	templates map[string]*template.Template
	loc       *time.Location
}

func NewHandler(registry *services.Registry, templates map[string]*template.Template, loc *time.Location) *Handler {
	return &Handler{
		registry:  registry,
		templates: templates,
		loc:       loc,
	}
}

func (h *Handler) HomePage(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	userSvc := h.registry.ForUser(uid)
	familySvc := h.registry.Family()
	renderHomePage(w, r, userSvc.Personal, familySvc, userSvc.Ideas, h.templates, h.loc)
}

func HomePageSingle(personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, templates map[string]*template.Template, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderHomePage(w, r, personalSvc, familySvc, ideaSvc, templates, loc)
	}
}

// Greeting returns a time-of-day greeting for the given time.
// 5-11: "Good morning", 12-17: "Good afternoon", 18-4: "Good evening".
func Greeting(now time.Time) string {
	hour := now.Hour()
	switch {
	case hour >= 5 && hour <= 11:
		return "Good morning"
	case hour >= 12 && hour <= 17:
		return "Good afternoon"
	default:
		return "Good evening"
	}
}

var planPrompts = []string{
	"Anything for today?",        // Sunday
	"What needs doing?",          // Monday
	"What matters today?",        // Tuesday
	"Three things?",              // Wednesday
	"What would make today good?", // Thursday
	"Last stretch of the week",   // Friday
	"Anything for today?",        // Saturday
}

func renderHomePage(w http.ResponseWriter, r *http.Request, personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, templates map[string]*template.Template, loc *time.Location) {
	personalItems, err := personalSvc.List()
	if err != nil {
		slog.Error("homepage personal list", "error", err)
	}
	familyItems, err := familySvc.List()
	if err != nil {
		slog.Error("homepage family list", "error", err)
	}
	allIdeas, err := ideaSvc.List()
	if err != nil {
		slog.Error("homepage ideas list", "error", err)
	}

	untriaged, untriagedCount := filterAndCountUntriaged(allIdeas, 3)

	now := time.Now().In(loc)

	// Build completed items from both personal and family lists.
	completedItems := toCompletedItems(personalItems)
	completedItems = append(completedItems, toCompletedItems(familyItems)...)

	velocity := insights.WeeklyVelocity(completedItems, now)
	streakDays, totalCompleted := insights.Streak(completedItems, now)
	milestone := insights.MilestoneBadge(totalCompleted)

	// Build tag info from all sources for cross-section aggregation.
	var tagInfos []insights.TagInfo
	for _, it := range personalItems {
		tagInfos = append(tagInfos, insights.TagInfo{Tags: it.Tags, Type: string(it.Type), Done: it.Done})
	}
	for _, it := range familyItems {
		tagInfos = append(tagInfos, insights.TagInfo{Tags: it.Tags, Type: string(it.Type), Done: it.Done})
	}
	for _, idea := range allIdeas {
		if idea.Status != "converted" {
			tagInfos = append(tagInfos, insights.TagInfo{Tags: idea.Tags, Type: "idea", Done: false})
		}
	}
	tagSummaries := insights.TopN(insights.TagAggregation(tagInfos), 5)

	// Daily planner data.
	today := now.Format("2006-01-02")
	personalPlanned := personalSvc.ListPlanned(today)
	familyPlanned := familySvc.ListPlanned(today)
	personalCarriedOver := personalSvc.ListOverdue(today)
	familyCarriedOver := familySvc.ListOverdue(today)

	// Unplanned tasks for the picker (open, not done, not planned, not carried over).
	personalExclude := make(map[string]bool)
	for _, it := range personalPlanned {
		personalExclude[it.Slug] = true
	}
	for _, it := range personalCarriedOver {
		personalExclude[it.Slug] = true
	}
	var unplannedPersonal []tracker.Item
	for _, it := range personalItems {
		if it.Type == tracker.TaskType && !it.Done && !personalExclude[it.Slug] {
			unplannedPersonal = append(unplannedPersonal, it)
		}
	}

	familyExclude := make(map[string]bool)
	for _, it := range familyPlanned {
		familyExclude[it.Slug] = true
	}
	for _, it := range familyCarriedOver {
		familyExclude[it.Slug] = true
	}
	var unplannedFamily []tracker.Item
	for _, it := range familyItems {
		if it.Type == tracker.TaskType && !it.Done && !familyExclude[it.Slug] {
			unplannedFamily = append(unplannedFamily, it)
		}
	}

	// Auto-promote: merge carried-over items into planned lists.
	personalCarriedCount := len(personalCarriedOver)
	familyCarriedCount := len(familyCarriedOver)
	personalPlanned = append(personalPlanned, personalCarriedOver...)
	familyPlanned = append(familyPlanned, familyCarriedOver...)

	sortPlanItems(personalPlanned)
	sortPlanItems(familyPlanned)
	sortByPriority(unplannedPersonal)
	sortByPriority(unplannedFamily)

	planDone := 0
	planTotal := len(personalPlanned) + len(familyPlanned)
	for _, it := range personalPlanned {
		if it.Done {
			planDone++
		}
	}
	for _, it := range familyPlanned {
		if it.Done {
			planDone++
		}
	}

	data := auth.TemplateData(r)
	data["Title"] = "Home"
	data["Greeting"] = Greeting(now)
	data["DateLabel"] = formatDateLabel(now)
	data["Today"] = today
	data["PersonalPlanned"] = personalPlanned
	data["FamilyPlanned"] = familyPlanned
	data["PersonalCarriedCount"] = personalCarriedCount
	data["FamilyCarriedCount"] = familyCarriedCount
	data["CarriedOverCount"] = personalCarriedCount + familyCarriedCount
	data["UnplannedPersonal"] = unplannedPersonal
	data["UnplannedFamily"] = unplannedFamily
	data["PlanDoneCount"] = planDone
	data["PlanTotalCount"] = planTotal
	data["PlanAllDone"] = planTotal > 0 && planDone == planTotal
	data["PlanPrompt"] = planPrompts[now.Weekday()]
	// Build set of planned slugs to exclude from summary cards.
	plannedSlugs := make(map[string]bool)
	for _, it := range personalPlanned {
		plannedSlugs[it.Slug] = true
	}
	familyPlannedSlugs := make(map[string]bool)
	for _, it := range familyPlanned {
		familyPlannedSlugs[it.Slug] = true
	}

	data["PersonalTasks"] = topTasksExcluding(personalItems, 5, plannedSlugs)
	data["PersonalTaskCount"] = countOpenTasks(personalItems)
	data["FamilyTasks"] = topTasksExcluding(familyItems, 5, familyPlannedSlugs)
	data["FamilyTaskCount"] = countOpenTasks(familyItems)
	data["Goals"] = activeGoals(personalItems)
	data["UntriagedIdeas"] = untriaged
	data["UntriagedCount"] = untriagedCount
	data["TotalIdeaCount"] = len(allIdeas)
	data["InsightLine"] = velocity
	data["StreakDays"] = streakDays
	data["TotalCompleted"] = totalCompleted
	data["MilestoneBadge"] = milestone
	data["TagSummaries"] = tagSummaries

	if msgKey := r.URL.Query().Get("msg"); msgKey != "" {
		if flashMsg := planFlashMessages[msgKey]; flashMsg != "" {
			data["FlashMsg"] = flashMsg
		}
	}

	if err := templates["homepage.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
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

func topTasksExcluding(items []tracker.Item, n int, exclude map[string]bool) []tracker.Item {
	var tasks []tracker.Item
	for _, it := range items {
		if it.Type == tracker.TaskType && !it.Done && !exclude[it.Slug] {
			tasks = append(tasks, it)
		}
	}
	slices.SortFunc(tasks, func(a, b tracker.Item) int {
		pa, pb := tracker.PriorityWeight[a.Priority], tracker.PriorityWeight[b.Priority]
		if pa != pb {
			return pa - pb
		}
		return 0
	})
	if len(tasks) > n {
		tasks = tasks[:n]
	}
	return tasks
}

func activeGoals(items []tracker.Item) []tracker.Item {
	var goals []tracker.Item
	for _, it := range items {
		if it.Type == tracker.GoalType && !it.Done {
			goals = append(goals, it)
		}
	}
	return goals
}

func filterAndCountUntriaged(allIdeas []ideas.Idea, n int) ([]ideas.Idea, int) {
	var preview []ideas.Idea
	count := 0
	for _, idea := range allIdeas {
		if idea.Status == "untriaged" {
			if count < n {
				preview = append(preview, idea)
			}
			count++
		}
	}
	return preview, count
}

// resolveServices returns (personalSvc, familySvc) for the current request context.
func (h *Handler) resolveServices(r *http.Request) (*tracker.Service, *tracker.Service) {
	uid := auth.UserID(r.Context())
	userSvc := h.registry.ForUser(uid)
	return userSvc.Personal, h.registry.Family()
}

var planFlashMessages = map[string]string{
	"plan-set":            "Added to today's plan.",
	"plan-cleared":        "Removed from plan.",
	"plan-completed":      "Nice one -- task completed.",
	"plan-bulk-set":       "Tasks added to today's plan.",
	"carried-cleared":     "Carried-over tasks dropped.",
	"plan-reordered":      "Plan order updated.",
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
	personal, family := h.resolveServices(r)
	today := time.Now().In(h.loc).Format("2006-01-02")
	clearOverdue(personal, today)
	clearOverdue(family, today)
	http.Redirect(w, r, "/?msg=carried-cleared", http.StatusSeeOther)
}

func clearOverdue(svc *tracker.Service, today string) {
	for _, it := range svc.ListOverdue(today) {
		_ = svc.ClearPlanned(it.Slug)
	}
}

// serviceForList returns the tracker service for the given list name.
func (h *Handler) serviceForList(r *http.Request, list string) *tracker.Service {
	personal, family := h.resolveServices(r)
	switch list {
	case "todos", "personal":
		return personal
	case "family":
		return family
	}
	return nil
}

// APIListPlan handles GET /api/v1/plan?date=YYYY-MM-DD.
func APIListPlan(personalSvc, familySvc *tracker.Service, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date == "" {
			date = time.Now().In(loc).Format("2006-01-02")
		}

		personal := personalSvc.ListPlanned(date)
		family := familySvc.ListPlanned(date)
		overdue := personalSvc.ListOverdue(date)
		overdue = append(overdue, familySvc.ListOverdue(date)...)

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"date":     date,
			"personal": planItemsToAPI(personal, "personal"),
			"family":   planItemsToAPI(family, "family"),
			"overdue":  planItemsToAPI(overdue, ""),
		})
	}
}

// APISetPlan handles PUT /api/v1/plan/{slug}.
func APISetPlan(personalSvc, familySvc *tracker.Service, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			body.Date = time.Now().In(loc).Format("2006-01-02")
		}

		var svc *tracker.Service
		switch body.List {
		case "personal", "todos":
			svc = personalSvc
		case "family":
			svc = familySvc
		default:
			http.Error(w, "Invalid list", http.StatusBadRequest)
			return
		}

		if err := svc.SetPlanned(slug, body.Date); err != nil {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APIClearPlan handles DELETE /api/v1/plan/{slug}.
func APIClearPlan(personalSvc, familySvc *tracker.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")

		var body struct {
			List string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		var svc *tracker.Service
		switch body.List {
		case "personal", "todos":
			svc = personalSvc
		case "family":
			svc = familySvc
		default:
			http.Error(w, "Invalid list", http.StatusBadRequest)
			return
		}

		if err := svc.ClearPlanned(slug); err != nil {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// SingleUserPlanHandlers holds plan handlers for single-user mode.
type SingleUserPlanHandlers struct {
	personalSvc *tracker.Service
	familySvc   *tracker.Service
	loc         *time.Location
}

func NewSingleUserPlanHandlers(personalSvc, familySvc *tracker.Service, loc *time.Location) *SingleUserPlanHandlers {
	return &SingleUserPlanHandlers{personalSvc: personalSvc, familySvc: familySvc, loc: loc}
}

func (h *SingleUserPlanHandlers) serviceForList(list string) *tracker.Service {
	switch list {
	case "todos", "personal":
		return h.personalSvc
	case "family":
		return h.familySvc
	}
	return nil
}

func (h *SingleUserPlanHandlers) SetPlanned(w http.ResponseWriter, r *http.Request) {
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
	svc := h.serviceForList(list)
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

func (h *SingleUserPlanHandlers) ClearPlanned(w http.ResponseWriter, r *http.Request) {
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
	svc := h.serviceForList(list)
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

func (h *SingleUserPlanHandlers) CompletePlanned(w http.ResponseWriter, r *http.Request) {
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
	svc := h.serviceForList(list)
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

func (h *SingleUserPlanHandlers) ReorderPlanned(w http.ResponseWriter, r *http.Request) {
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
	svc := h.serviceForList(list)
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

func (h *SingleUserPlanHandlers) ClearCarriedOver(w http.ResponseWriter, r *http.Request) {
	today := time.Now().In(h.loc).Format("2006-01-02")
	clearOverdue(h.personalSvc, today)
	clearOverdue(h.familySvc, today)
	http.Redirect(w, r, "/?msg=carried-cleared", http.StatusSeeOther)
}

func (h *SingleUserPlanHandlers) BulkSetPlanned(w http.ResponseWriter, r *http.Request) {
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
	svc := h.serviceForList(list)
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
