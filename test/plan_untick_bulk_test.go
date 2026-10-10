package test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/tracker"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Ticking a house project marks it done everywhere: Done for the digest and
// Status for the house page, from the plan and from the house page alike.
func TestHouseTickSetsStatus(t *testing.T) {
	const id = "fnc3pnt1"
	cases := map[string]struct {
		path string
		form url.Values
	}{
		"plan tick":       {"/plan/" + id + "/complete", url.Values{"list": {"house"}}},
		"house page tick": {"/house/projects/" + id + "/complete", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var path string
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				path = cfg.HouseProjectsPath
				seedFile(t, path, "# House\n\n- [ ] Paint the fence [planned: 2026-10-11] [status: active] [id: "+id+"]\n")
			})
			if rr := env.post(t, tc.path, tc.form); rr.Code != http.StatusSeeOther {
				t.Fatalf("POST %s: status %d: %s", tc.path, rr.Code, rr.Body.String())
			}
			it := itemsByID(t, path)[id]
			if !it.Done || it.Status != "done" || it.Completed == "" {
				t.Errorf("after %s: done %v, status %q, completed %q; want done, status done and a date", name, it.Done, it.Status, it.Completed)
			}
		})
	}
}

// planFiles seeds one planned task in each list the plan shows and returns
// the paths: todos (personal), family and house.
func planFiles(t *testing.T, cfg *config.Config, line func(id string) string) map[string]string {
	t.Helper()
	paths := map[string]string{
		"todos":  filepath.Join(cfg.UserDataDir, "1", "personal.md"),
		"family": cfg.FamilyPath,
		"house":  cfg.HouseProjectsPath,
	}
	seedFile(t, paths["todos"], "# Personal\n\n"+line(planIDs["todos"]))
	seedFile(t, paths["family"], "# Family\n\n"+line(planIDs["family"]))
	seedFile(t, paths["house"], "# House\n\n"+line(planIDs["house"]))
	return paths
}

var planIDs = map[string]string{"todos": "t0d0pln1", "family": "fmlpln01", "house": "hsplnd01"}

// Unticking from the plan reopens the task in place: the planned date and
// plan order stay, and a house project goes back to todo.
func TestPlanUncomplete(t *testing.T) {
	today := time.Now().In(time.Local).Format("2006-01-02")
	for list, id := range planIDs {
		t.Run(list, func(t *testing.T) {
			var paths map[string]string
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				paths = planFiles(t, cfg, func(id string) string {
					return "- [x] Sort it [completed: " + today + "] [planned: " + today + "] [plan-order: 2] [status: done] [id: " + id + "]\n"
				})
			})
			if rr := env.post(t, "/plan/"+id+"/uncomplete", url.Values{"list": {list}}); rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "msg=plan-uncompleted") {
				t.Fatalf("uncomplete: status %d, location %q", rr.Code, rr.Header().Get("Location"))
			}
			it := itemsByID(t, paths[list])[id]
			if it.Done || it.Completed != "" || it.Planned != today || it.PlanOrder != 2 || it.Status != "todo" {
				t.Errorf("after untick: %+v; want open, planned %s, order 2, status todo", it, today)
			}
		})
	}

	t.Run("rejects", func(t *testing.T) {
		env := newSeededAppEnv(t, func(cfg *config.Config) {
			planFiles(t, cfg, func(id string) string { return "- [x] Sort it [id: " + id + "]\n" })
		})
		for name, tc := range map[string]struct {
			path string
			list string
		}{
			"unknown list": {"/plan/" + planIDs["todos"] + "/uncomplete", "work"},
			"missing item": {"/plan/n0n3x1st/uncomplete", "todos"},
			"wrong list":   {"/plan/" + planIDs["family"] + "/uncomplete", "todos"},
		} {
			if rr := env.post(t, tc.path, url.Values{"list": {tc.list}}); rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status %d, want 400", name, rr.Code)
			}
		}
	})
}

// Bulk plan actions work on a selection spanning the three lists, and a bad
// selection writes nothing at all.
func TestPlanBulkActions(t *testing.T) {
	loc := time.Local
	today := time.Now().In(loc).Format("2006-01-02")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	all := "todos:" + planIDs["todos"] + ", family:" + planIDs["family"] + ",house:" + planIDs["house"]
	open := func(id string) string {
		return "- [ ] Sort it [planned: " + today + "] [plan-order: 1] [status: todo] [id: " + id + "]\n"
	}

	cases := map[string]struct {
		path  string
		flash string
		check func(it tracker.Item) bool
	}{
		"complete": {"/plan/bulk/complete", "3 tasks done.", func(it tracker.Item) bool {
			return it.Done && it.Completed != "" && it.Status == "done"
		}},
		"tomorrow": {"/plan/bulk/tomorrow", "3 tasks moved to tomorrow.", func(it tracker.Item) bool {
			return it.Planned == tomorrow && it.PlanOrder == 0 && !it.Done
		}},
		"drop": {"/plan/bulk/clear", "3 tasks removed from the plan.", func(it tracker.Item) bool {
			return it.Planned == "" && it.PlanOrder == 0 && it.DeletedAt == ""
		}},
		"trash": {"/plan/bulk/delete", "3 tasks moved to trash.", func(it tracker.Item) bool {
			return it.DeletedAt != ""
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var paths map[string]string
			env := newSeededAppEnv(t, func(cfg *config.Config) { paths = planFiles(t, cfg, open) })
			rr := env.post(t, tc.path, url.Values{"items": {all}})
			if rr.Code != http.StatusSeeOther {
				t.Fatalf("POST %s: status %d: %s", tc.path, rr.Code, rr.Body.String())
			}
			for list, id := range planIDs {
				if it := itemsByID(t, paths[list])[id]; !tc.check(it) {
					t.Errorf("%s item after %s: %+v", list, name, it)
				}
			}
			if page := env.get(t, rr.Header().Get("Location")); !strings.Contains(page, tc.flash) {
				t.Errorf("page after %s lacks the flash %q", name, tc.flash)
			}
		})
	}

	rejects := map[string]string{
		"empty":           "",
		"malformed id":    "todos:" + planIDs["todos"] + ",family:pay-rego",
		"no list":         planIDs["todos"],
		"unknown list":    "todos:" + planIDs["todos"] + ",work:" + planIDs["family"],
		"missing item":    "todos:" + planIDs["todos"] + ",family:n0n3x1st",
		"item elsewhere":  "todos:" + planIDs["todos"] + ",todos:" + planIDs["family"],
		"trashed item":    "house:" + planIDs["house"] + ",todos:" + planIDs["todos"],
		"tag in an entry": "todos:" + planIDs["todos"] + "] [tags: x",
	}
	for name, items := range rejects {
		t.Run("rejects "+name, func(t *testing.T) {
			var paths map[string]string
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				paths = planFiles(t, cfg, open)
				if name == "trashed item" {
					seedFile(t, paths["house"], "# House\n\n- [ ] Sort it [deleted: "+today+"] [id: "+planIDs["house"]+"]\n")
				}
			})
			before := map[string]string{}
			for list, p := range paths {
				before[list] = readFile(t, p)
			}
			for _, path := range []string{"/plan/bulk/complete", "/plan/bulk/tomorrow", "/plan/bulk/clear", "/plan/bulk/delete"} {
				if rr := env.post(t, path, url.Values{"items": {items}}); rr.Code != http.StatusBadRequest {
					t.Errorf("POST %s: status %d, want 400", path, rr.Code)
				}
			}
			for list, p := range paths {
				if readFile(t, p) != before[list] {
					t.Errorf("%s file changed by a rejected selection", list)
				}
			}
		})
	}
}

// A task named twice in a selection, even under the list's other name, is
// acted on and counted once.
func TestPlanBulkCountsEachTaskOnce(t *testing.T) {
	id := planIDs["todos"]
	env := newSeededAppEnv(t, func(cfg *config.Config) {
		planFiles(t, cfg, func(id string) string { return "- [ ] Sort it [id: " + id + "]\n" })
	})
	rr := env.post(t, "/plan/bulk/complete", url.Values{"items": {"personal:" + id + ",todos:" + id + ",todos:" + id}})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	if page := env.get(t, rr.Header().Get("Location")); !strings.Contains(page, "1 task done.") {
		t.Error("flash does not count the task once")
	}
}
