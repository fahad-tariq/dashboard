package test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/fahad/dashboard/web"
)

// The page holds one SSE connection, on <body>, which no swap replaces.
// Live containers refresh by morphing, so their rows keep their state.
func TestLiveContainersMorphUnderOneConnection(t *testing.T) {
	h, _ := renderRouter(t)
	live := regexp.MustCompile(`<div class="[^"]*"[^>]*\sdata-live[\s>][^>]*>`)
	for _, path := range []string{"/", "/todos", "/family", "/goals", "/ideas", "/house"} {
		body := renderPage(t, h, path)
		if n := strings.Count(body, "sse-connect="); n != 1 || !strings.Contains(body, `<body hx-ext="sse,morph" sse-connect="/events">`) {
			t.Errorf("%s: want exactly one sse-connect, on <body>; found %d", path, n)
		}
		tags := live.FindAllString(body, -1)
		if len(tags) != 1 {
			t.Fatalf("%s: %d live containers, want 1", path, len(tags))
		}
		for _, want := range []string{`hx-swap="morph"`, `hx-select="[data-live]"`, `hx-target="this"`, `hx-push-url="false"`} {
			if !strings.Contains(tags[0], want) {
				t.Errorf("%s: live container lacks %s", path, want)
			}
		}
		// The SSE trigger sits on a child whose attributes never change; on
		// the container, each morph would add another SSE listener.
		if strings.Contains(tags[0], "hx-trigger") {
			t.Errorf("%s: the live container itself carries hx-trigger", path)
		}
		refresh := regexp.MustCompile(`<span class="live-refresh" id="live-refresh" hidden hx-get="[^"]+" hx-trigger="sse:changed:[^"]+" hx-target="closest \[data-live\]"`)
		if !refresh.MatchString(body) {
			t.Errorf("%s: no live-refresh trigger inside the container", path)
		}
	}
}

// Focus after a change finds rows and their lists by id.
func TestRowsAndListsHaveIDs(t *testing.T) {
	h, _ := renderRouter(t)
	tag := regexp.MustCompile(`<[a-z]+\s[^>]*\sdata-row(-list)?[\s>][^>]*>`)
	for _, path := range []string{"/", "/todos", "/goals", "/ideas", "/house"} {
		body := renderPage(t, h, path)
		found := tag.FindAllString(body, -1)
		if len(found) == 0 {
			t.Errorf("%s: no rows", path)
		}
		for _, el := range found {
			if !regexp.MustCompile(`\sid="[^"]+"`).MatchString(el) {
				t.Errorf("%s: row or list without an id: %s", path, el)
			}
		}
	}
}

// Morph swaps replaced the flags that used to discard refreshes.
func TestNoRefreshSuppressionFlags(t *testing.T) {
	for _, name := range []string{"tracker.js", "planner.js", "house.js", "live.js", "shortcuts.js", "dialog.js"} {
		src, err := fs.ReadFile(web.StaticFS, "static/"+name)
		if err != nil {
			t.Fatal(err)
		}
		for _, flag := range []string{"planDetailExpanded", "trackerExpandedItems", "planDragInProgress", "shouldSwap = false"} {
			if strings.Contains(string(src), flag) {
				t.Errorf("%s still uses %s", name, flag)
			}
		}
	}
}
