package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/config"
)

// dueRouter builds the no-auth router with personal and family lists whose
// deadlines sit the given number of days from today.
func dueRouter(t *testing.T, personal, family func(day func(int) string) string) (http.Handler, *config.Config) {
	t.Helper()
	return renderRouterWith(t, func(seeds map[string]string, today string) {
		base, err := time.Parse("2006-01-02", today)
		if err != nil {
			t.Fatal(err)
		}
		day := func(n int) string { return base.AddDate(0, 0, n).Format("2006-01-02") }
		seeds["PERSONAL_PATH"] = "# Personal\n\n" + personal(day)
		seeds["FAMILY_PATH"] = "# Family\n\n" + family(day)
	})
}

func render(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rr.Code)
	}
	return rr.Body.String()
}

var dueWidgetRe = regexp.MustCompile(`(?s)<section class="homepage-card" aria-labelledby="widget-todos-due-title">.*?</section>`)

// The "Due soon" widget lists open tasks from both lists that are overdue or
// due within three days, overdue first, and leaves out done tasks, goals and
// later deadlines.
func TestDueSoonWidget(t *testing.T) {
	h, _ := dueRouter(t,
		func(day func(int) string) string {
			return "- [ ] Pay rego [deadline: " + day(3) + "]\n" +
				"- [ ] Lodge tax [deadline: " + day(-2) + "] [planned: " + day(0) + "]\n" +
				"- [ ] Book dentist [deadline: " + day(0) + "]\n" +
				"- [ ] Renew insurance [deadline: " + day(4) + "]\n" +
				"- [x] Return library books [deadline: " + day(0) + "] [completed: " + day(-1) + "]\n" +
				"- [ ] Run 100km [goal: 10/100 km] [deadline: " + day(1) + "]\n" +
				"- [ ] Trashed task [deadline: " + day(0) + "] [deleted: " + day(-1) + "]\n"
		},
		func(day func(int) string) string {
			return "- [ ] Sign permission slip [deadline: " + day(1) + "]\n"
		})
	body := render(t, h, "/")
	widget := dueWidgetRe.FindString(body)
	if widget == "" {
		t.Fatal("no Due soon widget")
	}
	titles := regexp.MustCompile(`data-row-focus>([^<]+)</a>`).FindAllStringSubmatch(widget, -1)
	var got []string
	for _, m := range titles {
		got = append(got, m[1])
	}
	want := []string{"Lodge tax", "Book dentist", "Sign permission slip", "Pay rego"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("rows %q, want %q", got, want)
	}
	for _, want := range []string{
		`<span class="stat-value stat-danger">4</span> <span class="meta-dim">due</span>`,
		`<span class="widget-meta widget-meta-danger"><span aria-hidden="true">overdue 2d</span><span class="sr-only">Overdue by 2 days</span></span>`,
		`<a href="/family#item-sign-permission-slip" data-row-focus>Sign permission slip</a> <span class="meta-dim">Family</span>`,
		// Lodge tax is already in today's plan (carried over counts too).
		`<span class="badge badge-planned">planned</span>`,
		`aria-label="Plan Book dentist for today">plan today</button>`,
		`<input type="hidden" name="list" value="family"><input type="hidden" name="slug" value="sign-permission-slip">`,
	} {
		if !strings.Contains(widget, want) {
			t.Errorf("widget lacks %s\n%s", want, widget)
		}
	}
	if strings.Contains(widget, `name="date"`) {
		t.Error("the plan action must leave the date to the server")
	}
}

func TestDueSoonWidgetHiddenWhenNothingIsDue(t *testing.T) {
	h, _ := dueRouter(t,
		func(day func(int) string) string {
			return "- [ ] Renew insurance [deadline: " + day(4) + "]\n- [ ] No date\n"
		},
		func(func(int) string) string { return "" })
	if strings.Contains(render(t, h, "/"), "widget-todos-due") {
		t.Error("Due soon widget shown with nothing due")
	}
}

// "plan today" posts without a date; the server plans the task for its own
// today, the plan shows it, and the widget row stays with a "planned" badge
// in place of the button.
func TestDueSoonPlanToday(t *testing.T) {
	h, cfg := dueRouter(t,
		func(day func(int) string) string { return "- [ ] Book dentist [deadline: " + day(1) + "]\n" },
		func(func(int) string) string { return "" })
	req := httptest.NewRequest("POST", "/plan/set", strings.NewReader(url.Values{"slug": {"book-dentist"}, "list": {"todos"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want the replayed homepage", rr.Code)
	}
	today := time.Now().In(cfg.Location).Format("2006-01-02")
	data, err := os.ReadFile(cfg.PersonalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[planned: "+today+"]") {
		t.Errorf("not planned for %s:\n%s", today, data)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="plan-todos-book-dentist"`) {
		t.Error("task not in today's plan")
	}
	widget := dueWidgetRe.FindString(body)
	for _, want := range []string{`id="widget-todos-due-todos-book-dentist" data-row`, `<span class="badge badge-planned">planned</span>`} {
		if !strings.Contains(widget, want) {
			t.Errorf("widget lacks %s\n%s", want, widget)
		}
	}
	if strings.Contains(widget, "plan today</button>") {
		t.Error("button still shown once planned")
	}
}

// Rows show the due badge with the full label for screen readers; the plan
// row shows it only once the task is due today or overdue.
func TestDueBadges(t *testing.T) {
	h, _ := dueRouter(t,
		func(day func(int) string) string {
			return "- [ ] Book dentist [deadline: " + day(0) + "] [planned: " + day(0) + "]\n" +
				"- [ ] Pay rego [deadline: " + day(5) + "] [planned: " + day(0) + "]\n" +
				"- [ ] Lodge tax [deadline: " + day(-1) + "] [planned: " + day(-3) + "]\n" +
				"- [ ] Run 100km [goal: 10/100 km] [deadline: " + day(-1) + "]\n"
		},
		func(func(int) string) string { return "" })
	todos := render(t, h, "/todos")
	if !regexp.MustCompile(`<time class="badge badge-due badge-due-danger" datetime="\d{4}-\d{2}-\d{2}"><span aria-hidden="true">overdue 1d</span><span class="sr-only">Overdue by 1 day</span></time>`).MatchString(todos) {
		t.Error("todos row lacks the overdue badge")
	}
	if !strings.Contains(todos, `<span class="sr-only">Due today</span>`) {
		t.Error("todos row lacks the due-today badge")
	}
	for _, want := range []string{`<label for="task-deadline" class="date-field-label">Due</label>`, `id="item-pay-rego-deadline" value="`, `data-action="clear-date" data-clear="item-pay-rego-deadline"`} {
		if !strings.Contains(todos, want) {
			t.Errorf("todos page lacks %s", want)
		}
	}

	home := render(t, h, "/")
	planRow := func(slug string) string {
		return regexp.MustCompile(`(?s)id="plan-todos-` + slug + `".*?<div class="plan-item-detail"`).FindString(home)
	}
	if !strings.Contains(planRow("book-dentist"), `<span aria-hidden="true">due today</span>`) {
		t.Error("plan row due today lacks the label")
	}
	if strings.Contains(planRow("pay-rego"), "badge-due") {
		t.Error("plan row due in 5 days shows a due label")
	}
	if !regexp.MustCompile(`<span class="plan-item-date">from 3 days ago &middot; <time class="badge badge-due badge-due-danger"`).MatchString(planRow("lodge-tax")) {
		t.Errorf("carried and overdue row lacks the merged label:\n%s", planRow("lodge-tax"))
	}

	goals := render(t, h, "/goals")
	if !strings.Contains(goals, `<span class="sr-only">Overdue by 1 day</span>`) || strings.Contains(goals, "due 20") {
		t.Error("goal does not use the due helper")
	}
}

// Only an edit that carries the deadline field changes it: moving a task and
// editing a house project keep it.
func TestDeadlineSurvivesMoveAndPartialEdits(t *testing.T) {
	cases := map[string]struct {
		path    string
		form    url.Values
		file    func(*config.Config) string
		wantRow string
	}{
		"move to family": {
			path:    "/todos/book-dentist/move",
			file:    func(c *config.Config) string { return c.FamilyPath },
			wantRow: "- [ ] Book dentist [added: 2026-09-01] [deadline: 2026-11-20]",
		},
		"house project edit": {
			path:    "/house/projects/fix-the-gate/edit",
			form:    url.Values{"title": {"Fix the gate"}, "body": {"New hinges"}, "tags": {"exterior"}},
			file:    func(c *config.Config) string { return c.HouseProjectsPath },
			wantRow: "- [ ] Fix the gate [added: 2026-09-01] [deadline: 2026-11-20] [tags: exterior] [status: todo]",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h, cfg := renderRouterWith(t, func(seeds map[string]string, _ string) {
				seeds["PERSONAL_PATH"] = "# Personal\n\n- [ ] Book dentist [added: 2026-09-01] [deadline: 2026-11-20]\n"
				seeds["HOUSE_PROJECTS_PATH"] = "# House\n\n- [ ] Fix the gate [added: 2026-09-01] [deadline: 2026-11-20] [status: todo]\n"
			})
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusSeeOther {
				t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
			}
			data, err := os.ReadFile(tc.file(cfg))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.wantRow+"\n") {
				t.Errorf("file lacks %q:\n%s", tc.wantRow, data)
			}
		})
	}
}
