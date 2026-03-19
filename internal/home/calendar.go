package home

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

// CalendarDay holds planned tasks for a single day.
type CalendarDay struct {
	Date      string // YYYY-MM-DD
	Label     string // day number, e.g. "19"
	Weekday   string // short weekday, e.g. "Wed"
	IsToday   bool
	Personal  []tracker.Item
	Family    []tracker.Item
	TaskCount int
}

// CalendarPage handles GET /plan/calendar in auth-enabled mode.
func (h *Handler) CalendarPage(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	userSvc := h.registry.ForUser(uid)
	familySvc := h.registry.Family()
	renderCalendarPage(w, r, userSvc.Personal, familySvc, h.templates)
}

// CalendarPageSingle returns a handler for GET /plan/calendar in single-user mode.
func CalendarPageSingle(personalSvc, familySvc *tracker.Service, _ *ideas.Service, templates map[string]*template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderCalendarPage(w, r, personalSvc, familySvc, templates)
	}
}

func renderCalendarPage(w http.ResponseWriter, r *http.Request, personalSvc, familySvc *tracker.Service, templates map[string]*template.Template) {
	view := r.URL.Query().Get("view")
	if view != "month" {
		view = "week"
	}

	now := time.Now()
	today := now.Format("2006-01-02")

	dateStr := r.URL.Query().Get("date")
	ref := now
	if dateStr != "" {
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			ref = t
		}
	}

	var start, end time.Time
	if view == "month" {
		start = time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
		end = start.AddDate(0, 1, -1)
	} else {
		// ISO week: Monday to Sunday.
		wd := ref.Weekday()
		if wd == time.Sunday {
			wd = 7
		}
		start = ref.AddDate(0, 0, -int(wd)+1)
		end = start.AddDate(0, 0, 6)
	}

	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")

	personal := personalSvc.ListPlannedRange(startStr, endStr)
	family := familySvc.ListPlannedRange(startStr, endStr)

	days := BuildCalendarDays(personal, family, start, end, today)

	// Navigation.
	var prev, next time.Time
	var header string
	if view == "month" {
		prev = start.AddDate(0, -1, 0)
		next = start.AddDate(0, 1, 0)
		header = ref.Format("January 2006")
	} else {
		prev = start.AddDate(0, 0, -7)
		next = start.AddDate(0, 0, 7)
		endLabel := end
		if start.Month() == end.Month() {
			header = start.Format("2") + " \u2013 " + endLabel.Format("2 Jan 2006")
		} else {
			header = start.Format("2 Jan") + " \u2013 " + endLabel.Format("2 Jan 2006")
		}
	}

	data := auth.TemplateData(r)
	data["Title"] = "Calendar"
	data["View"] = view
	data["Days"] = days
	data["Header"] = header
	data["Today"] = today
	data["PrevDate"] = prev.Format("2006-01-02")
	data["NextDate"] = next.Format("2006-01-02")

	if err := templates["calendar.html"].ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("rendering calendar", "error", err)
	}
}

// BuildCalendarDays groups planned items into day buckets across the date range.
func BuildCalendarDays(personal, family []tracker.Item, start, end time.Time, today string) []CalendarDay {
	personalByDate := groupByDate(personal)
	familyByDate := groupByDate(family)

	var days []CalendarDay
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		p := personalByDate[ds]
		f := familyByDate[ds]
		days = append(days, CalendarDay{
			Date:      ds,
			Label:     d.Format("2"),
			Weekday:   d.Format("Mon"),
			IsToday:   ds == today,
			Personal:  p,
			Family:    f,
			TaskCount: len(p) + len(f),
		})
	}
	return days
}

func groupByDate(items []tracker.Item) map[string][]tracker.Item {
	m := make(map[string][]tracker.Item)
	for _, it := range items {
		if it.Planned != "" {
			m[it.Planned] = append(m[it.Planned], it)
		}
	}
	return m
}
