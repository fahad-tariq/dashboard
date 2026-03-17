package home

import (
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/tracker"
)

type Handler struct {
	registry  *services.Registry
	templates map[string]*template.Template
}

func NewHandler(registry *services.Registry, templates map[string]*template.Template) *Handler {
	return &Handler{
		registry:  registry,
		templates: templates,
	}
}

func (h *Handler) HomePage(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	userSvc := h.registry.ForUser(uid)
	familySvc := h.registry.Family()
	renderHomePage(w, r, userSvc.Personal, familySvc, userSvc.Ideas, h.templates)
}

func HomePageSingle(personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderHomePage(w, r, personalSvc, familySvc, ideaSvc, templates)
	}
}

func renderHomePage(w http.ResponseWriter, r *http.Request, personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, templates map[string]*template.Template) {
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

	now := time.Now()

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

	data := auth.TemplateData(r)
	data["Title"] = "Home"
	data["PersonalTasks"] = topTasks(personalItems, 5)
	data["PersonalTaskCount"] = countOpenTasks(personalItems)
	data["FamilyTasks"] = topTasks(familyItems, 5)
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

	if err := templates["homepage.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering homepage", "error", err)
	}
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

func topTasks(items []tracker.Item, n int) []tracker.Item {
	var tasks []tracker.Item
	for _, it := range items {
		if it.Type == tracker.TaskType && !it.Done {
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
