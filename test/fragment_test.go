package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/sse"
)

// htmxPost sends a form POST the way htmx does when htmx is true, with referer
// as the page it came from.
func htmxPost(t *testing.T, h http.Handler, path, referer string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

type flashTrigger struct {
	Flash struct {
		Key     string `json:"key"`
		Message string `json:"message"`
		Error   bool   `json:"error"`
		Undo    string `json:"undo"`
	} `json:"dash:flash"`
}

func readTrigger(t *testing.T, rr *httptest.ResponseRecorder) flashTrigger {
	t.Helper()
	var got flashTrigger
	raw := rr.Header().Get("HX-Trigger")
	if raw == "" {
		t.Fatal("no HX-Trigger header")
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("HX-Trigger %q is not JSON: %v", raw, err)
	}
	return got
}

// An htmx mutation gets the updated page back in the same response instead
// of a redirect, so one mutation is one request.
func TestHTMXMutationReturnsFragment(t *testing.T) {
	h, _ := renderRouter(t)
	tests := map[string]struct {
		path, referer string
		form          url.Values
		live          string // the live container the response must hold
		key           string
		undo          string
	}{
		"tracker complete": {path: "/todos/plan-weekend-hike/complete", referer: "/todos", live: `class="tracker-page"`, key: "task-completed"},
		"tracker trash":    {path: "/todos/plan-weekend-hike/delete", referer: "/todos", live: `class="tracker-page"`, key: "item-deleted", undo: "/todos/plan-weekend-hike/restore"},
		"plan complete":    {path: "/plan/renew-passport/complete", form: url.Values{"list": {"todos"}}, live: `class="homepage-page"`, key: "plan-completed"},
		"plan clear":       {path: "/plan/clear", form: url.Values{"list": {"family"}, "slug": {"organise-school-pickup-roster"}}, live: `class="homepage-page"`, key: "plan-cleared"},
		"idea triage":      {path: "/ideas/home-weather-station/triage", form: url.Values{"action": {"park"}}, live: `class="ideas-page"`, key: "idea-triaged"},
		"idea trash":       {path: "/ideas/learn-to-sail/delete", live: `class="ideas-page"`, key: "idea-deleted", undo: "/ideas/learn-to-sail/restore"},
		"maintenance log":  {path: "/house/maintenance/clean-gutters/log", form: url.Values{"note": {"done"}}, live: `class="house-page"`, key: "completion-logged"},
		"project trash":    {path: "/house/projects/paint-the-back-fence/delete", live: `class="house-page"`, key: "item-deleted", undo: "/house/projects/paint-the-back-fence/restore"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rr := htmxPost(t, h, tc.path, tc.referer, tc.form, true)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d, want 200; body %q", rr.Code, rr.Body.String())
			}
			body, _ := io.ReadAll(rr.Body)
			if !strings.Contains(string(body), tc.live) {
				t.Errorf("response lacks the live container %s", tc.live)
			}
			if !slices.Contains(rr.Header().Values("Vary"), "HX-Request") {
				t.Errorf("Vary = %q, want HX-Request", rr.Header().Values("Vary"))
			}
			if rr.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", rr.Header().Get("Cache-Control"))
			}
			got := readTrigger(t, rr)
			if got.Flash.Key != tc.key || got.Flash.Message == "" || got.Flash.Error {
				t.Errorf("flash = %+v, want key %q with a message", got.Flash, tc.key)
			}
			if got.Flash.Undo != tc.undo {
				t.Errorf("undo = %q, want %q", got.Flash.Undo, tc.undo)
			}
			if !strings.Contains(rr.Header().Get("Dash-Revisions"), "=") {
				t.Errorf("Dash-Revisions = %q, want module revisions", rr.Header().Get("Dash-Revisions"))
			}
		})
	}
}

// Plain form posts keep the redirect, and htmx failures come back as errors
// the page can show rather than as a page.
func TestFragmentResponsesLeavePlainPostsAndErrors(t *testing.T) {
	h, _ := renderRouter(t)

	rr := htmxPost(t, h, "/todos/plan-weekend-hike/complete", "/todos", nil, false)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("HX-Trigger") != "" {
		t.Errorf("plain post: status %d, HX-Trigger %q; want 303 and none", rr.Code, rr.Header().Get("HX-Trigger"))
	}

	rr = htmxPost(t, h, "/todos/no-such-task/complete", "/todos", nil, true)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "not found") {
		t.Errorf("missing item: status %d body %q; want 400 with the reason", rr.Code, rr.Body.String())
	}

	// A protocol-relative referer is reduced to its path and replayed
	// locally; nothing points the page off the site.
	rr = htmxPost(t, h, "/todos/plan-weekend-hike/complete", "//evil.example/x", nil, true)
	if rr.Header().Get("Location") != "" || strings.Contains(rr.Body.String(), "evil.example") {
		t.Errorf("foreign referer: status %d Location %q; want a local replay", rr.Code, rr.Header().Get("Location"))
	}
}

// Each debounced change event carries its module's revision, so a tab that
// already has the result of its own write can skip the echo.
func TestDebouncedChangedCarriesRevision(t *testing.T) {
	b := sse.NewBroker()
	ch := make(chan string, 16)
	b.Subscribe(ch)
	defer b.Unsubscribe(ch)

	publish := b.DebouncedChanged(20 * time.Millisecond)
	publish("todos")
	publish("todos")
	if got := b.Revisions()["todos"]; got != 2 {
		t.Fatalf("revision after two writes = %d, want 2", got)
	}
	select {
	case msg := <-ch:
		if msg != "event: changed:todos\ndata: todos 2\n\n" {
			t.Errorf("event = %q, want revision 2 in the data", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}

// The planner's own fetches (calendar drag, reorder) get 204 and refresh
// themselves; they never go through the fragment replay.
func TestPlannerXHRGetsNoContent(t *testing.T) {
	h, _ := renderRouter(t)
	tests := map[string]struct {
		path string
		form url.Values
	}{
		"set":     {"/plan/set", url.Values{"slug": {"plan-weekend-hike"}, "list": {"todos"}, "date": {"2026-10-09"}}},
		"reorder": {"/plan/reorder", url.Values{"slugs": {"renew-passport"}, "list": {"todos"}}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusNoContent {
				t.Errorf("status %d, want 204", rr.Code)
			}
		})
	}
}

// With auth on, the replayed page is rendered for the signed-in user. A
// request whose session has gone is sent to the login page as a whole-page
// navigation; replaying it would swap the login form into the list.
func TestFragmentReplayKeepsTheSession(t *testing.T) {
	h, _, _ := authRouter(t, []testUser{{"owner@test.com", "correct-horse-battery"}}, map[string]string{
		"1/personal.md": "# Personal\n\n- [ ] Water plants [added: 2026-10-01]\n",
	})
	cookie := login(t, h, "owner@test.com", "correct-horse-battery")

	post := func(c *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/todos/water-plants/complete", nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("Referer", "/todos")
		req.Header.Set("HX-Current-URL", "http://example.com/todos")
		if c != nil {
			req.AddCookie(c)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	rr := post(cookie)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `class="tracker-page"`) || strings.Contains(rr.Body.String(), `action="/login"`) {
		t.Fatalf("signed in: status %d; want the /todos page, not the login form", rr.Code)
	}
	if got := readTrigger(t, rr); got.Flash.Key != "task-completed" {
		t.Errorf("flash key = %q, want task-completed", got.Flash.Key)
	}

	rr = post(nil)
	if rr.Header().Get("HX-Redirect") != "/login?expired=1&next=%2Ftodos" || strings.Contains(rr.Body.String(), "<form") {
		t.Errorf("no session: status %d HX-Redirect %q; want a navigation to /login and no page", rr.Code, rr.Header().Get("HX-Redirect"))
	}
}
