package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

// Trash on the idea detail page offers undo like every other trash action,
// without htmx: a plain POST with no confirm, whose redirect to the list
// carries the restore path, which the layout offers as an undo button.
func TestIdeaDetailTrashOffersUndo(t *testing.T) {
	h, _ := renderRouter(t)
	body := renderPage(t, h, "/ideas/"+weatherStation)

	form := regexp.MustCompile(`<form[^>]*action="/ideas/w3th3rst/delete"[^>]*>`).FindString(body)
	if form == "" {
		t.Fatal("no trash form on the idea detail page")
	}
	if strings.Contains(form, "data-confirm") || strings.Contains(form, "hx-") {
		t.Errorf("trash form should be a plain POST with no confirm: %s", form)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/ideas/"+weatherStation+"/delete", nil))
	loc := rr.Header().Get("Location")
	if rr.Code != http.StatusSeeOther || !strings.Contains(loc, "undo=%2Fideas%2F"+weatherStation+"%2Frestore") {
		t.Fatalf("trash = %d to %q, want a 303 carrying the restore path", rr.Code, loc)
	}
	list := renderPage(t, h, loc)
	undo := `<form method="POST" action="/ideas/w3th3rst/restore" class="flash-undo">`
	if !strings.Contains(list, undo) {
		t.Errorf("list page after trash has no undo form %q", undo)
	}
}

// The undo button only ever posts to a local restore route, whatever the
// query string says.
func TestFlashUndoOnlyForLocalRestorePaths(t *testing.T) {
	h, _ := renderRouter(t)
	tests := map[string]struct {
		undo string
		want bool
	}{
		"restore route":     {undo: "/ideas/x/restore", want: true},
		"purge route":       {undo: "/ideas/x/purge"},
		"protocol relative": {undo: "//evil.example/restore"},
		"absolute URL":      {undo: "https://evil.example/restore"},
		"no flash message":  {undo: "/ideas/x/restore"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := "/ideas?msg=idea-deleted&undo=" + url.QueryEscape(tc.undo)
			if name == "no flash message" {
				path = "/ideas?undo=" + url.QueryEscape(tc.undo)
			}
			got := strings.Contains(renderPage(t, h, path), `class="flash-undo"`)
			if got != tc.want {
				t.Errorf("undo form shown = %v, want %v", got, tc.want)
			}
		})
	}
}
