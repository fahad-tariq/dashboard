package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/itemid"
)

// apiStep is one request in the golden script. Route is the chi pattern
// with the item ID route written {id}; it names the golden file.
type apiStep struct {
	route, path, body string
}

// apiScript runs in order against one copy of the e2e fixtures, so later
// steps see earlier changes. It covers every /api/v1 route, and through them
// every branch of the serialisers: goals, house fields, images, deadlines,
// sub-steps, trashed items and carried-over plan items.
func apiScript(today, yesterday string) []apiStep {
	return []apiStep{
		{route: "GET /todos", path: "/api/v1/todos"},
		{route: "GET /todos/{id}", path: "/api/v1/todos/pssp0rt1?list=personal"},
		{route: "GET /todos/{id}", path: "/api/v1/todos/bks12gl0?list=todos"},
		{route: "GET /todos/{id}", path: "/api/v1/todos/h1k3wknd?list=personal"},
		{route: "GET /todos/{id}", path: "/api/v1/todos/zzzzzzzz?list=personal"},
		{route: "GET /todos/{id}", path: "/api/v1/todos/pssp0rt1"},

		{route: "POST /todos", path: "/api/v1/todos", body: `{"title":"Buy milk #groceries ! High","list":"personal"}`},
		{route: "POST /todos", path: "/api/v1/todos", body: `{"id":"nw000001","title":"Book flights","list":"family","priority":"low","tags":["travel"]}`},
		{route: "POST /todos", path: "/api/v1/todos", body: `{"id":"nw000001","title":"Book flights again","list":"family"}`},
		{route: "POST /todos", path: "/api/v1/todos", body: `{"id":"aeiou123","title":"Bad ID","list":"personal"}`},
		{route: "POST /todos", path: "/api/v1/todos", body: `{"title":"#onlytags","list":"personal"}`},
		{route: "POST /todos", path: "/api/v1/todos", body: `{"title":"No list"}`},

		{route: "PUT /todos/{id}", path: "/api/v1/todos/pssp0rt1", body: `{"list":"personal","images":["passport.jpg|Old photo"],"deadline":"2026-11-30"}`},
		{route: "PUT /todos/{id}", path: "/api/v1/todos/pssp0rt1", body: `{"list":"personal","deadline":"soon"}`},
		{route: "PUT /todos/{id}", path: "/api/v1/todos/zzzzzzzz", body: `{"list":"personal","title":"Nothing"}`},

		{route: "POST /todos/{id}/complete", path: "/api/v1/todos/bnkc4ll1/complete", body: `{"list":"personal"}`},
		{route: "POST /todos/{id}/uncomplete", path: "/api/v1/todos/bnkc4ll1/uncomplete", body: `{"list":"personal"}`},
		{route: "PUT /todos/{id}/priority", path: "/api/v1/todos/bnkc4ll2/priority", body: `{"list":"personal","priority":"medium"}`},
		{route: "PUT /todos/{id}/priority", path: "/api/v1/todos/zzzzzzzz/priority", body: `{"list":"personal","priority":"high"}`},
		{route: "PUT /todos/{id}/tags", path: "/api/v1/todos/bnkc4ll2/tags", body: `{"list":"personal","tags":["admin","bank]"]}`},
		{route: "PUT /todos/{id}/tags", path: "/api/v1/todos/bnkc4ll2/tags", body: `{"list":"personal","tags":["a","b","c","d","e","f","g","h","i","j","k"]}`},

		{route: "POST /todos/{id}/substeps", path: "/api/v1/todos/h1k3wknd/substeps", body: `{"list":"personal","text":"Pack snacks"}`},
		{route: "POST /todos/{id}/substeps", path: "/api/v1/todos/h1k3wknd/substeps", body: `{"list":"personal","text":"  "}`},
		{route: "PUT /todos/{id}/substeps/{index}", path: "/api/v1/todos/h1k3wknd/substeps/1", body: `{"list":"personal"}`},
		{route: "PUT /todos/{id}/substeps/{index}", path: "/api/v1/todos/h1k3wknd/substeps/9", body: `{"list":"personal"}`},
		{route: "DELETE /todos/{id}/substeps/{index}", path: "/api/v1/todos/h1k3wknd/substeps/2", body: `{"list":"personal"}`},

		{route: "DELETE /todos/{id}", path: "/api/v1/todos/bnkc4ll2", body: `{"list":"personal"}`},
		{route: "POST /todos", path: "/api/v1/todos", body: `{"id":"bnkc4ll2","title":"Call the bank","list":"personal"}`},

		{route: "PUT /plan/{id}", path: "/api/v1/plan/txrtrn01", body: `{"list":"personal","date":"` + yesterday + `"}`},
		{route: "PUT /plan/{id}", path: "/api/v1/plan/h1k3wknd", body: `{"list":"personal"}`},
		{route: "PUT /plan/{id}", path: "/api/v1/plan/pssp0rt1", body: `{"list":"todos","date":"` + today + `"}`},
		{route: "PUT /plan/{id}", path: "/api/v1/plan/rstr0001", body: `{"list":"family"}`},
		{route: "PUT /plan/{id}", path: "/api/v1/plan/fnc3pnt1", body: `{"list":"house"}`},
		{route: "PUT /plan/{id}", path: "/api/v1/plan/fnc3pnt1", body: `{"list":"work"}`},
		{route: "PUT /plan/{id}", path: "/api/v1/plan/zzzzzzzz", body: `{"list":"personal"}`},

		{route: "GET /plan", path: "/api/v1/plan"},
		{route: "GET /plan", path: "/api/v1/plan?date=2026-01-15"},
		{route: "GET /plan", path: "/api/v1/plan?date=tomorrow"},

		{route: "POST /plan/reorder", path: "/api/v1/plan/reorder", body: `{"list":"personal","ids":["pssp0rt1","h1k3wknd"]}`},
		{route: "POST /plan/reorder", path: "/api/v1/plan/reorder", body: `{"list":"personal","ids":["pssp0rt1","zzzzzzzz"]}`},
		{route: "POST /plan/reorder", path: "/api/v1/plan/reorder", body: `{"list":"personal","ids":[]}`},
		{route: "POST /plan/reorder", path: "/api/v1/plan/reorder", body: `{"list":"work","ids":["pssp0rt1"]}`},
		{route: "DELETE /plan/{id}", path: "/api/v1/plan/pssp0rt1", body: `{"list":"personal"}`},
		{route: "DELETE /plan/{id}", path: "/api/v1/plan/zzzzzzzz", body: `{"list":"personal"}`},
		{route: "DELETE /plan/{id}", path: "/api/v1/plan/pssp0rt1", body: `{`},
		{route: "POST /plan/clear-carried", path: "/api/v1/plan/clear-carried"},

		{route: "GET /ideas", path: "/api/v1/ideas"},
		{route: "POST /ideas", path: "/api/v1/ideas", body: `{"id":"nw000002","title":"Rainwater tank","tags":["garden"],"body":"https://example.com/tanks"}`},
		{route: "POST /ideas", path: "/api/v1/ideas", body: `{"id":"nw000002","title":"Rainwater tank"}`},
		{route: "POST /ideas", path: "/api/v1/ideas", body: `{"title":""}`},
		{route: "PUT /ideas/{id}/triage", path: "/api/v1/ideas/wthr5tn1/triage", body: `{"action":"park"}`},
		{route: "PUT /ideas/{id}/triage", path: "/api/v1/ideas/wthr5tn1/triage", body: `{"action":"sell"}`},
		{route: "PUT /ideas/{id}/triage", path: "/api/v1/ideas/zzzzzzzz/triage", body: `{"action":"park"}`},
		{route: "POST /ideas/{id}/research", path: "/api/v1/ideas/s41l1ng1/research", body: `{"content":"Two clubs run beginner courses."}`},
		{route: "POST /ideas/{id}/research", path: "/api/v1/ideas/zzzzzzzz/research", body: `{"content":"Nothing"}`},
	}
}

// TestAPIGolden pins the status and JSON body of every /api/v1 route,
// against testdata/api/<method>_<path>.golden. Run with -update to accept a
// change, and explain it in the plan's working notes.
func TestAPIGolden(t *testing.T) {
	var today, yesterday string
	env := newSeededAppEnv(t, func(cfg *config.Config) {
		now := time.Now().In(cfg.Location)
		today, yesterday = now.Format("2006-01-02"), now.AddDate(0, 0, -1).Format("2006-01-02")
		for src, dst := range map[string]string{
			"users/1/personal.md": filepath.Join(cfg.UserDataDir, "1", "personal.md"),
			"users/1/ideas.md":    filepath.Join(cfg.UserDataDir, "1", "ideas.md"),
			"family.md":           cfg.FamilyPath,
			"house-projects.md":   cfg.HouseProjectsPath,
			"maintenance.md":      cfg.MaintenancePath,
		} {
			data, err := os.ReadFile(filepath.Join("..", "e2e", "fixtures", src))
			if err != nil {
				t.Fatal(err)
			}
			writeSeed(t, dst, string(data))
		}
	})
	script := apiScript(today, yesterday)

	// Every API route has at least one step.
	scripted := map[string]bool{}
	for _, s := range script {
		scripted[s.route] = true
	}
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if r, ok := strings.CutPrefix(route, "/api/v1"); ok {
			r = method + " " + strings.ReplaceAll(r, itemid.Route, "{id}")
			if !scripted[r] {
				t.Errorf("route %s has no golden step", r)
			}
		}
		return nil
	}
	if err := chi.Walk(env.h.(chi.Routes), walk); err != nil {
		t.Fatal(err)
	}

	known := knownIDs(t, script)
	dates := strings.NewReplacer(today, "<today>", yesterday, "<yesterday>")
	out := map[string]*bytes.Buffer{}
	var order []string
	for _, s := range script {
		method, _, _ := strings.Cut(s.route, " ")
		rr := env.api(t, method, s.path, s.body)
		name := goldenName(s.route)
		buf, ok := out[name]
		if !ok {
			buf = &bytes.Buffer{}
			out[name] = buf
			order = append(order, name)
		} else {
			buf.WriteString("\n")
		}
		fmt.Fprintf(buf, "### %s %s\n", method, dates.Replace(s.path))
		if s.body != "" {
			fmt.Fprintf(buf, "> %s\n", dates.Replace(s.body))
		}
		fmt.Fprintf(buf, "%d %s\n", rr.Code, rr.Header().Get("Content-Type"))
		buf.WriteString(normaliseAPIBody(t, rr.Body.Bytes(), known, today, yesterday))
	}
	for _, name := range order {
		assertGolden(t, filepath.Join("api", name), out[name].Bytes())
	}
}

// goldenName turns "PUT /todos/{id}/substeps/{index}" into
// "PUT_todos_id_substeps_index.golden".
func goldenName(route string) string {
	r := strings.NewReplacer(" /", "_", "/", "_", "{", "", "}", "")
	return r.Replace(route) + ".golden"
}

var idPattern = regexp.MustCompile(`[` + itemid.Alphabet + `]{8}`)

// knownIDs collects the IDs a golden may show as they are: those in the
// fixtures and those the script sends.
func knownIDs(t *testing.T, script []apiStep) map[string]bool {
	t.Helper()
	known := map[string]bool{}
	files, err := filepath.Glob(filepath.Join("..", "e2e", "fixtures", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	user, err := filepath.Glob(filepath.Join("..", "e2e", "fixtures", "users", "1", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range append(files, user...) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range regexp.MustCompile(`\[id: (\w+)\]`).FindAllStringSubmatch(string(data), -1) {
			known[m[1]] = true
		}
	}
	for _, s := range script {
		for _, id := range idPattern.FindAllString(s.path+s.body, -1) {
			known[id] = true
		}
	}
	return known
}

var (
	idField   = regexp.MustCompile(`"(id|from_idea|converted_to)":"([^"]*)"`)
	dateField = regexp.MustCompile(`"(added|completed|planned|deleted|date|today)":"(\d{4}-\d{2}-\d{2})"`)
)

// normaliseAPIBody replaces IDs the server made up and the dates of the
// run with placeholders, keeping field order, and indents the JSON.
// Deadlines are left alone: the script sets them to fixed dates.
func normaliseAPIBody(t *testing.T, body []byte, known map[string]bool, today, yesterday string) string {
	t.Helper()
	s := strings.TrimSpace(string(body))
	s = idField.ReplaceAllStringFunc(s, func(m string) string {
		parts := idField.FindStringSubmatch(m)
		if known[parts[2]] {
			return m
		}
		return `"` + parts[1] + `":"<new-id>"`
	})
	s = dateField.ReplaceAllStringFunc(s, func(m string) string {
		parts := dateField.FindStringSubmatch(m)
		switch parts[2] {
		case today:
			return `"` + parts[1] + `":"<today>"`
		case yesterday:
			return `"` + parts[1] + `":"<yesterday>"`
		}
		return m
	})
	if !json.Valid([]byte(s)) {
		return s + "\n"
	}
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(s), "", "  "); err != nil {
		t.Fatal(err)
	}
	return out.String() + "\n"
}
