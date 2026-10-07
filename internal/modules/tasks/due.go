package tasks

import (
	"cmp"
	"log/slog"
	"slices"
	"time"

	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/tracker"
)

// dueSoonDays is how far ahead the "Due soon" widget looks.
const dueSoonDays = 3

// dueList is one list the "Due soon" widget reads: its route and module ID
// (todos or family), the label its rows carry, and its service.
type dueList struct {
	list, context string
	svc           *tracker.Service
}

type dueRow struct {
	item tracker.Item
	list dueList
	due  insights.Due
}

// dueWidget lists the open tasks that are overdue or due within
// dueSoonDays, overdue first, then by date. Each row can be planned for
// today in one click; the plan itself is never changed automatically. It is
// hidden when nothing is due.
func dueWidget(lists []dueList, now time.Time) (module.WidgetData, bool) {
	rows := dueRows(lists, now)
	if len(rows) == 0 {
		return module.WidgetData{}, false
	}
	data := module.WidgetData{ID: "due", Title: "Due soon", Count: len(rows), CountLabel: "due"}
	for _, r := range rows {
		switch {
		case r.due.Days < 0:
			data.Severity = module.SeverityDanger
		case r.due.Days <= 2 && data.Severity == module.SeverityNone:
			data.Severity = module.SeverityWarning
		}
	}
	today := now.Format("2006-01-02")
	for _, r := range rows[:min(len(rows), 5)] {
		data.Items = append(data.Items, dueItem(r, today))
	}
	return data, true
}

// dueRows collects the due tasks of every list, sorted by days left.
func dueRows(lists []dueList, now time.Time) []dueRow {
	var rows []dueRow
	for _, l := range lists {
		items, err := l.svc.List()
		if err != nil {
			slog.Error("due widget", "list", l.list, "error", err)
			continue
		}
		for _, it := range items {
			if it.Type != tracker.TaskType || it.Done || it.Deadline == "" {
				continue
			}
			due := insights.DueLabel(it.Deadline, now)
			if due.Short != "" && due.Days <= dueSoonDays {
				rows = append(rows, dueRow{item: it, list: l, due: due})
			}
		}
	}
	slices.SortStableFunc(rows, func(a, b dueRow) int { return cmp.Compare(a.due.Days, b.due.Days) })
	return rows
}

// dueItem is one widget row with its "plan today" action, or a "planned"
// badge once the task is in today's plan (carried-over tasks count).
func dueItem(r dueRow, today string) module.WidgetItem {
	it := r.item
	action := &module.WidgetAction{
		// No date: the server plans for its own today, so a tab left open
		// past midnight still plans the right day.
		Path:   "/plan/set",
		Fields: map[string]string{"slug": it.Slug, "list": r.list.list},
		Text:   "plan today",
		Label:  "Plan " + it.Title + " for today",
	}
	if it.Planned != "" && it.Planned <= today {
		action.Done = "planned"
	}
	return module.WidgetItem{
		ID:       r.list.list + "-" + it.Slug,
		Label:    it.Title,
		URL:      "/" + r.list.list + "#item-" + it.Slug,
		Priority: it.Priority,
		Context:  r.list.context,
		Meta:     &module.WidgetMeta{Text: r.due.Short, Label: r.due.Full, Level: r.due.Level},
		Action:   action,
	}
}
