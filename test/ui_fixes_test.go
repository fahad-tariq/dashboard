package test

import (
	"regexp"
	"strings"
	"testing"
)

// Done tasks show struck through in the calendar week view; like done plan
// rows on the homepage, they must not be draggable to another day.
func TestCalendarDoneTasksAreNotDraggable(t *testing.T) {
	h, _ := renderRouterWith(t, func(seeds map[string]string, today string) {
		seeds["PERSONAL_PATH"] += "- [x] Posted the parcel [added: 2026-09-01] [completed: " + today + "] [planned: " + today + "]\n"
	})
	body := renderPage(t, h, "/plan/calendar?view=week")

	task := regexp.MustCompile(`(?s)<div class="calendar-task"[^>]*>.*?</div>`)
	var sawDone, sawOpen bool
	for _, m := range task.FindAllString(body, -1) {
		draggable := strings.Contains(m, `draggable="true"`)
		switch {
		case strings.Contains(m, "Posted the parcel"):
			sawDone = true
			if draggable {
				t.Errorf("done task is draggable: %s", m)
			}
		case strings.Contains(m, "Renew passport"):
			sawOpen = true
			if !draggable {
				t.Errorf("open task is not draggable: %s", m)
			}
		}
	}
	if !sawDone || !sawOpen {
		t.Fatalf("calendar is missing a seeded task (done %v, open %v)", sawDone, sawOpen)
	}
}

// Bulk trash can be restored, so its flash is a status message, not an error.
func TestBulkTrashFlashIsNotAnError(t *testing.T) {
	h, _ := renderRouter(t)
	for _, path := range []string{"/todos?msg=bulk-deleted", "/ideas?msg=bulk-deleted"} {
		body := renderPage(t, h, path)
		if !strings.Contains(body, `class="flash-msg" data-error="false" role="status"`) {
			t.Errorf("%s: bulk trash flash is not a plain status message", path)
		}
	}
}

// Trash on the idea detail page offers undo like every other trash action:
// no confirm, and an htmx request so the fragment middleware's toast carries
// the undo. The response is the ideas list, so it replaces <main>.
func TestIdeaDetailTrashOffersUndo(t *testing.T) {
	h, _ := renderRouter(t)
	body := renderPage(t, h, "/ideas/home-weather-station")

	form := regexp.MustCompile(`<form[^>]*action="/ideas/home-weather-station/delete"[^>]*>`).FindString(body)
	if form == "" {
		t.Fatal("no trash form on the idea detail page")
	}
	if strings.Contains(form, "data-confirm") {
		t.Errorf("trash form still asks for confirmation: %s", form)
	}
	for _, want := range []string{`hx-boost="true"`, `hx-target="#main"`, `hx-select="#main"`, `hx-swap="outerHTML"`, `hx-push-url="/ideas"`} {
		if !strings.Contains(form, want) {
			t.Errorf("trash form lacks %s: %s", want, form)
		}
	}
}
