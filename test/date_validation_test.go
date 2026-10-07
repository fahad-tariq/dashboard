package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every entry point that writes a planned date or a deadline rejects text
// that is not a real YYYY-MM-DD date with a 400 and leaves the file alone, so
// a date field can never inject a tag. Valid dates are written, and an empty
// deadline clears it.
func TestDateEntryPointsValidate(t *testing.T) {
	const seed = "# Personal\n\n- [ ] Renew licence [added: 2026-09-01] [deadline: 2026-11-20]\n"
	type call struct {
		method, path string
		form         url.Values // a form post when set
		json         string     // an API request when set
	}
	cases := map[string]struct {
		call       call
		wantStatus int
		want       string // a substring the file must then hold; empty means unchanged
		wantNot    string // a substring the file must then lack
	}{
		"add goal rejects an injected deadline": {
			call:       call{method: "POST", path: "/todos/add-goal", form: url.Values{"title": {"Run 100km"}, "target": {"100"}, "deadline": {"x] [tags: y"}}},
			wantStatus: http.StatusBadRequest,
		},
		"add goal rejects an impossible date": {
			call:       call{method: "POST", path: "/todos/add-goal", form: url.Values{"title": {"Run 100km"}, "target": {"100"}, "deadline": {"2026-02-30"}}},
			wantStatus: http.StatusBadRequest,
		},
		"add goal accepts a real date": {
			call:       call{method: "POST", path: "/todos/add-goal", form: url.Values{"title": {"Run 100km"}, "target": {"100"}, "deadline": {"2026-12-31"}}},
			wantStatus: http.StatusSeeOther,
			want:       "Run 100km [goal: 0/100] [added: ",
		},
		"plan set rejects an injected date": {
			call:       call{method: "POST", path: "/plan/set", form: url.Values{"slug": {"renew-licence"}, "list": {"todos"}, "date": {"x] [tags: y"}}},
			wantStatus: http.StatusBadRequest,
		},
		"plan set accepts a real date": {
			call:       call{method: "POST", path: "/plan/set", form: url.Values{"slug": {"renew-licence"}, "list": {"todos"}, "date": {"2026-10-09"}}},
			wantStatus: http.StatusSeeOther,
			want:       "[planned: 2026-10-09]",
		},
		"plan bulk set rejects an injected date": {
			call:       call{method: "POST", path: "/plan/bulk/set", form: url.Values{"slugs": {"renew-licence"}, "list": {"todos"}, "date": {"x] [tags: y"}}},
			wantStatus: http.StatusBadRequest,
		},
		"plan bulk set rejects a short date": {
			call:       call{method: "POST", path: "/plan/bulk/set", form: url.Values{"slugs": {"renew-licence"}, "list": {"todos"}, "date": {"2026-1-9"}}},
			wantStatus: http.StatusBadRequest,
		},
		"API plan set rejects an injected date": {
			call:       call{method: "PUT", path: "/api/v1/plan/renew-licence", json: `{"list":"personal","date":"x] [tags: y"}`},
			wantStatus: http.StatusBadRequest,
		},
		"edit form rejects an injected deadline": {
			call:       call{method: "POST", path: "/todos/renew-licence/edit", form: url.Values{"title": {"Renew licence"}, "deadline": {"x] [tags: y"}}},
			wantStatus: http.StatusBadRequest,
		},
		"edit form sets a deadline": {
			call:       call{method: "POST", path: "/todos/renew-licence/edit", form: url.Values{"title": {"Renew licence"}, "deadline": {"2026-10-30"}}},
			wantStatus: http.StatusSeeOther,
			want:       "[deadline: 2026-10-30]",
		},
		"edit form with an empty deadline clears it": {
			call:       call{method: "POST", path: "/todos/renew-licence/edit", form: url.Values{"title": {"Renew licence"}, "deadline": {""}}},
			wantStatus: http.StatusSeeOther,
			wantNot:    "[deadline:",
		},
		"edit form without the field keeps the deadline": {
			call:       call{method: "POST", path: "/todos/renew-licence/edit", form: url.Values{"title": {"Renew licence"}, "body": {"Bring the old card"}}},
			wantStatus: http.StatusSeeOther,
			want:       "[deadline: 2026-11-20]",
		},
		"API edit rejects an injected deadline": {
			call:       call{method: "PUT", path: "/api/v1/todos/renew-licence", json: `{"list":"personal","deadline":"x] [tags: y"}`},
			wantStatus: http.StatusBadRequest,
		},
		"API edit sets a deadline": {
			call:       call{method: "PUT", path: "/api/v1/todos/renew-licence", json: `{"list":"personal","deadline":"2026-10-30"}`},
			wantStatus: http.StatusOK,
			want:       "[deadline: 2026-10-30]",
		},
		"API edit with an empty deadline clears it": {
			call:       call{method: "PUT", path: "/api/v1/todos/renew-licence", json: `{"list":"personal","deadline":""}`},
			wantStatus: http.StatusOK,
			wantNot:    "[deadline:",
		},
		"API edit without the field keeps the deadline": {
			call:       call{method: "PUT", path: "/api/v1/todos/renew-licence", json: `{"list":"personal","body":"Bring the old card"}`},
			wantStatus: http.StatusOK,
			want:       "[deadline: 2026-11-20]",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newAppEnv(t)
			file := filepath.Join(env.cfg.UserDataDir, "1", "personal.md")
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}

			var rr *httptest.ResponseRecorder
			if tc.call.form != nil {
				rr = env.post(t, tc.call.path, tc.call.form)
			} else {
				req := httptest.NewRequest(tc.call.method, tc.call.path, strings.NewReader(tc.call.json))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+env.token)
				rr = httptest.NewRecorder()
				env.h.ServeHTTP(rr, req)
			}
			if rr.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if strings.Contains(got, "tags: y") {
				t.Errorf("a date field injected a tag:\n%s", got)
			}
			if tc.want == "" && tc.wantNot == "" && got != seed {
				t.Errorf("file changed:\n%s", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("file lacks %q:\n%s", tc.want, got)
			}
			if tc.wantNot != "" && strings.Contains(got, tc.wantNot) {
				t.Errorf("file still holds %q:\n%s", tc.wantNot, got)
			}
		})
	}
}
