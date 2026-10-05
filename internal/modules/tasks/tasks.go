// Package tasks holds the two modules built on the tracker library: the
// owner's todos (with goals) and the shared family list. Both render the
// pages in web/templates/tracker/.
package tasks

import (
	"log/slog"
	"slices"
	"strconv"

	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/tracker"
)

// templates is the template directory both modules share.
const templates = "tracker"

// resync re-reads svc's file and reports whether it differed from the
// service's own last write.
func resync(svc *tracker.Service) func(int64) (bool, error) {
	return func(int64) (bool, error) { return svc.ResyncIfChanged() }
}

// taskWidget lists a list's open tasks, highest priority first, leaving out
// those already in today's plan. It is hidden when nothing is open.
func taskWidget(id, title, path string, svc *tracker.Service, today string) (module.WidgetData, bool) {
	items, err := svc.List()
	if err != nil {
		slog.Error("task widget", "list", path, "error", err)
		return module.WidgetData{}, false
	}
	planned := map[string]bool{}
	for _, it := range slices.Concat(svc.ListPlanned(today), svc.ListOverdue(today)) {
		planned[it.Slug] = true
	}
	var open []tracker.Item
	count := 0
	for _, it := range items {
		if it.Type != tracker.TaskType || it.Done {
			continue
		}
		count++
		if !planned[it.Slug] {
			open = append(open, it)
		}
	}
	if count == 0 {
		return module.WidgetData{}, false
	}
	slices.SortStableFunc(open, func(a, b tracker.Item) int {
		return tracker.PriorityWeight[a.Priority] - tracker.PriorityWeight[b.Priority]
	})
	data := module.WidgetData{ID: id, Title: title, Count: count, CountLabel: "open", Link: path}
	for _, it := range open[:min(len(open), 5)] {
		data.Items = append(data.Items, module.WidgetItem{Label: it.Title, URL: path + "#item-" + it.Slug, Priority: it.Priority})
	}
	return data, true
}

// formatNum prints whole numbers without a decimal point.
func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
