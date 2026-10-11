package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/config"
)

// api sends a JSON request with the bearer token.
func (e *contractEnv) api(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)
	return rr
}

// apiJSON sends a request, checks the status and decodes the response.
func (e *contractEnv) apiJSON(t *testing.T, method, path, body string, wantStatus int) map[string]any {
	t.Helper()
	rr := e.api(t, method, path, body)
	if rr.Code != wantStatus {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, rr.Code, wantStatus, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: decoding %q: %v", method, path, rr.Body.String(), err)
	}
	return out
}

// planEnv seeds the owner's personal list and the shared family and house
// lists with tasks planned for today and for yesterday (carried over).
func planEnv(t *testing.T) (*contractEnv, string, string) {
	t.Helper()
	var today, yesterday string
	env := newSeededAppEnv(t, func(cfg *config.Config) {
		now := time.Now().In(cfg.Location)
		today = now.Format("2006-01-02")
		yesterday = now.AddDate(0, 0, -1).Format("2006-01-02")
		writeSeed(t, filepath.Join(cfg.UserDataDir, "1", "personal.md"), "# Personal\n\n"+
			"- [ ] Today task [planned: "+today+"] [plan-order: 2] [id: tdz00001]\n"+
			"- [ ] Carried task [deadline: 2026-12-01] [planned: "+yesterday+"] [id: crr00001]\n"+
			"- [ ] Secret task [tags: private] [planned: "+today+"] [id: prv00001]\n")
		writeSeed(t, cfg.FamilyPath, "# Family\n\n- [ ] Family carried [planned: "+yesterday+"] [id: fmc00001]\n")
		writeSeed(t, cfg.HouseProjectsPath, "# House\n\n- [ ] House carried [status: todo] [planned: "+yesterday+"] [id: hsc00001]\n")
	})
	return env, today, yesterday
}

func writeSeed(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Every item the plan endpoint returns names its list, carried items keep
// the day they were planned for, and the response says what today is.
func TestAPIPlanItemsCarryListAndDates(t *testing.T) {
	env, today, yesterday := planEnv(t)
	resp := env.apiJSON(t, "GET", "/api/v1/plan", "", http.StatusOK)
	if resp["today"] != today {
		t.Errorf("today = %v, want %s", resp["today"], today)
	}
	wantLists := map[string]string{"tdz00001": "personal", "crr00001": "personal", "prv00001": "personal", "fmc00001": "family", "hsc00001": "house"}
	seen := map[string]bool{}
	for _, key := range []string{"personal", "family", "house", "overdue"} {
		items, _ := resp[key].([]any)
		for _, raw := range items {
			it, _ := raw.(map[string]any)
			id, _ := it["id"].(string)
			seen[id] = true
			if it["list"] != wantLists[id] {
				t.Errorf("%s item %s: list = %v, want %s", key, id, it["list"], wantLists[id])
			}
			if _, ok := it["deadline"]; !ok {
				t.Errorf("%s item %s has no deadline field", key, id)
			}
			if key == "overdue" && it["planned"] != yesterday {
				t.Errorf("overdue item %s: planned = %v, want %s", id, it["planned"], yesterday)
			}
		}
	}
	for id := range wantLists {
		if !seen[id] {
			t.Errorf("item %s missing from the plan (a private tag no longer hides it)", id)
		}
	}
}

func TestAPIPlanRequests(t *testing.T) {
	tests := map[string]struct {
		method, path, body string
		wantStatus         int
	}{
		"reorder with an unknown ID is 404":     {method: "POST", path: "/api/v1/plan/reorder", body: `{"list":"personal","ids":["tdz00001","zzzzzzzz"]}`, wantStatus: http.StatusNotFound},
		"reorder with known IDs is 200":         {method: "POST", path: "/api/v1/plan/reorder", body: `{"list":"personal","ids":["tdz00001","prv00001"]}`, wantStatus: http.StatusOK},
		"set with the personal alias":           {method: "PUT", path: "/api/v1/plan/crr00001", body: `{"list":"personal"}`, wantStatus: http.StatusOK},
		"set with todos":                        {method: "PUT", path: "/api/v1/plan/crr00001", body: `{"list":"todos"}`, wantStatus: http.StatusOK},
		"clear with the personal alias":         {method: "DELETE", path: "/api/v1/plan/crr00001", body: `{"list":"personal"}`, wantStatus: http.StatusOK},
		"clear with todos":                      {method: "DELETE", path: "/api/v1/plan/crr00001", body: `{"list":"todos"}`, wantStatus: http.StatusOK},
		"set on an unknown ID is 404":           {method: "PUT", path: "/api/v1/plan/zzzzzzzz", body: `{"list":"personal"}`, wantStatus: http.StatusNotFound},
		"clear with an unknown list is 400":     {method: "DELETE", path: "/api/v1/plan/crr00001", body: `{"list":"work"}`, wantStatus: http.StatusBadRequest},
		"set with an invalid date is 400":       {method: "PUT", path: "/api/v1/plan/crr00001", body: `{"list":"personal","date":"tomorrow"}`, wantStatus: http.StatusBadRequest},
		"plan for an invalid date is 400":       {method: "GET", path: "/api/v1/plan?date=x", wantStatus: http.StatusBadRequest},
		"plan for a valid date is 200":          {method: "GET", path: "/api/v1/plan?date=2026-01-01", wantStatus: http.StatusOK},
		"clear-carried answers with JSON":       {method: "POST", path: "/api/v1/plan/clear-carried", wantStatus: http.StatusOK},
		"set with a malformed body is JSON 400": {method: "PUT", path: "/api/v1/plan/crr00001", body: `{`, wantStatus: http.StatusBadRequest},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env, _, _ := planEnv(t)
			rr := env.api(t, tc.method, tc.path, tc.body)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type %q, want application/json", ct)
			}
		})
	}
}

// itemEnv seeds one task with sub-steps, an order and a private tag, and
// one trashed task.
func itemEnv(t *testing.T) *contractEnv {
	t.Helper()
	return newSeededAppEnv(t, func(cfg *config.Config) {
		writeSeed(t, filepath.Join(cfg.UserDataDir, "1", "personal.md"), "# Personal\n\n"+
			"- [ ] Pack for trip [tags: private] [planned: 2026-10-01] [plan-order: 3] [id: pck00001]\n"+
			"  Leave by nine.\n"+
			"  - [x] Passport\n"+
			"  - [ ] Charger\n"+
			"- [ ] Old errand [deleted: 2026-10-05] [id: dlt00001]\n")
	})
}

func TestAPIItemShape(t *testing.T) {
	env := itemEnv(t)

	t.Run("by ID carries sub-steps, plan order and no deleted marker", func(t *testing.T) {
		it := env.apiJSON(t, "GET", "/api/v1/todos/pck00001?list=personal", "", http.StatusOK)
		steps, _ := it["sub_steps"].([]any)
		want := []map[string]any{{"text": "Passport", "done": true}, {"text": "Charger", "done": false}}
		if len(steps) != len(want) {
			t.Fatalf("sub_steps = %v, want %v", it["sub_steps"], want)
		}
		for i, raw := range steps {
			step, _ := raw.(map[string]any)
			if step["text"] != want[i]["text"] || step["done"] != want[i]["done"] {
				t.Errorf("sub_steps[%d] = %v, want %v", i, step, want[i])
			}
		}
		if it["sub_steps_done"] != float64(1) || it["sub_steps_total"] != float64(2) {
			t.Errorf("counts = %v/%v, want 1/2", it["sub_steps_done"], it["sub_steps_total"])
		}
		if it["plan_order"] != float64(3) {
			t.Errorf("plan_order = %v, want 3", it["plan_order"])
		}
		if _, ok := it["deleted"]; ok {
			t.Errorf("open item carries deleted = %v", it["deleted"])
		}
	})

	t.Run("a trashed item says when it was deleted", func(t *testing.T) {
		it := env.apiJSON(t, "GET", "/api/v1/todos/dlt00001?list=personal", "", http.StatusOK)
		if it["deleted"] != "2026-10-05" {
			t.Errorf("deleted = %v, want 2026-10-05", it["deleted"])
		}
	})

	t.Run("an item with no sub-steps has an empty list", func(t *testing.T) {
		it := env.apiJSON(t, "GET", "/api/v1/todos/dlt00001?list=personal", "", http.StatusOK)
		steps, ok := it["sub_steps"].([]any)
		if !ok || len(steps) != 0 {
			t.Errorf("sub_steps = %#v, want []", it["sub_steps"])
		}
	})

	t.Run("a private item is listed", func(t *testing.T) {
		resp := env.apiJSON(t, "GET", "/api/v1/todos", "", http.StatusOK)
		items, _ := resp["personal"].([]any)
		if !slices.ContainsFunc(items, func(raw any) bool {
			it, _ := raw.(map[string]any)
			return it["id"] == "pck00001"
		}) {
			t.Error("private-tagged item missing from GET /todos")
		}
	})
}

// The service cleans tag and image values before they reach its cache, so
// what the API reads back matches what the file holds.
func TestAPITagsMatchFileAfterWrite(t *testing.T) {
	tests := map[string]struct {
		method, path, body string
		field              string
		want               []string
	}{
		"set tags":    {method: "PUT", path: "/api/v1/todos/pck00001/tags", body: `{"list":"personal","tags":["a]b"," c ","!high","x,y"]}`, field: "tags", want: []string{"ab", "c", "xy"}},
		"edit tags":   {method: "PUT", path: "/api/v1/todos/pck00001", body: `{"list":"personal","tags":["[one]","  "]}`, field: "tags", want: []string{"one"}},
		"edit images": {method: "PUT", path: "/api/v1/todos/pck00001", body: `{"list":"personal","images":["a].png"," b.png"]}`, field: "images", want: []string{"a.png", "b.png"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := itemEnv(t)
			if rr := env.api(t, tc.method, tc.path, tc.body); rr.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
			}
			// The file is what a restart would load.
			data, err := os.ReadFile(filepath.Join(env.cfg.UserDataDir, "1", "personal.md"))
			if err != nil {
				t.Fatal(err)
			}
			wantTag := "[" + tc.field + ": " + strings.Join(tc.want, ", ") + "]"
			if !strings.Contains(string(data), wantTag) {
				t.Fatalf("file lacks %q:\n%s", wantTag, data)
			}
			it := env.apiJSON(t, "GET", "/api/v1/todos/pck00001?list=personal", "", http.StatusOK)
			raw, _ := it[tc.field].([]any)
			var got []string
			for _, v := range raw {
				s, _ := v.(string)
				got = append(got, s)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s from the API = %q, want %q (as in the file)", tc.field, got, tc.want)
			}
		})
	}
}
