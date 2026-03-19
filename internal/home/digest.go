package home

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/tracker"
)

// DigestPage handles GET /digest in auth-enabled mode.
func (h *Handler) DigestPage(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	userSvc := h.registry.ForUser(uid)
	familySvc := h.registry.Family()
	renderDigestPage(w, r, userSvc.Personal, familySvc, userSvc.Ideas, h.templates, h.loc)
}

// DigestPageSingle returns a handler for GET /digest in single-user mode.
func DigestPageSingle(personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, templates map[string]*template.Template, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderDigestPage(w, r, personalSvc, familySvc, ideaSvc, templates, loc)
	}
}

func renderDigestPage(w http.ResponseWriter, r *http.Request, personalSvc, familySvc *tracker.Service, ideaSvc *ideas.Service, templates map[string]*template.Template, loc *time.Location) {
	period := insights.ParseDigestPeriod(r.URL.Query().Get("period"))
	now := time.Now().In(loc)

	personalItems, err := personalSvc.List()
	if err != nil {
		slog.Error("digest personal list", "error", err)
	}
	familyItems, err := familySvc.List()
	if err != nil {
		slog.Error("digest family list", "error", err)
	}
	allIdeas, err := ideaSvc.List()
	if err != nil {
		slog.Error("digest ideas list", "error", err)
	}

	// Merge personal and family tracker items into digest items.
	digestItems := make([]insights.DigestItem, 0, len(personalItems)+len(familyItems)+len(allIdeas))
	for _, it := range append(personalItems, familyItems...) {
		digestItems = append(digestItems, insights.DigestItem{
			Added:     it.Added,
			Completed: it.Completed,
			Done:      it.Done,
			Tags:      it.Tags,
			Type:      string(it.Type),
		})
	}
	for _, idea := range allIdeas {
		digestItems = append(digestItems, insights.DigestItem{
			Added: idea.Added,
			Done:  false,
			Tags:  idea.Tags,
			Type:  "idea",
		})
	}

	digest := insights.Digest(digestItems, period, now)

	// Compute all-time idea totals (period filtering unavailable for these).
	convertedCount := countIdeaStatus(allIdeas, "converted")
	triagedCount := len(allIdeas) - countIdeaStatus(allIdeas, "untriaged")

	data := auth.TemplateData(r)
	data["Title"] = "Digest"
	data["Digest"] = digest
	data["Period"] = string(period)
	data["ConvertedIdeas"] = convertedCount
	data["TriagedIdeas"] = triagedCount

	if err := templates["digest.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering digest", "error", err)
	}
}

func countIdeaStatus(allIdeas []ideas.Idea, status string) int {
	count := 0
	for _, idea := range allIdeas {
		if idea.Status == status {
			count++
		}
	}
	return count
}

