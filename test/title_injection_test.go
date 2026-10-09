package test

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

// injected is appended to titles: tags no user types on purpose.
const injected = " [planned: 2026-01-01] [tags: injected] [id: bcdfghjk]"

// No input path can set inline metadata through a title, a tag or a goal
// unit: every value a form or the API sends is written into an item line,
// where the parsers would read it back as a tag.
func TestTitlesCannotInjectTags(t *testing.T) {
	type file int
	const (
		personal file = iota
		houseProjects
		maintenance
		ideaFile
	)
	cases := map[string]struct {
		path string
		form url.Values
		json string // an API request when set
		file file
	}{
		"todos add":                                 {path: "/todos/add", form: url.Values{"title": {"Buy milk" + injected}}, file: personal},
		"todos add, newline in title":               {path: "/todos/add", form: url.Values{"title": {"Buy milk\n- [ ] Evil [tags: injected]"}}, file: personal},
		"todos add, tag closes early":               {path: "/todos/add", form: url.Values{"title": {"Buy milk"}, "tags": {"ok] [planned: 2026-01-01"}}, file: personal},
		"todos edit":                                {path: "/todos/seed-task/edit", form: url.Values{"title": {"Seed task" + injected}}, file: personal},
		"todos bulk tag":                            {path: "/todos/bulk/tag", form: url.Values{"slugs": {"seed-task"}, "tag": {"ok] [planned: 2026-01-01"}}, file: personal},
		"todos add, priority in a tag":              {path: "/todos/add", form: url.Values{"title": {"Buy milk"}, "tags": {"!high"}}, file: personal},
		"todos bulk tag, priority":                  {path: "/todos/bulk/tag", form: url.Values{"slugs": {"seed-task"}, "tag": {"!high"}}, file: personal},
		"todos add, priority rebuilt in a tag":      {path: "/todos/add", form: url.Values{"title": {"Buy milk"}, "tags": {"!hi!highgh"}}, file: personal},
		"todos bulk tag, priority rebuilt":          {path: "/todos/bulk/tag", form: url.Values{"slugs": {"seed-task"}, "tag": {"!hi!highgh"}}, file: personal},
		"todos edit, priority rebuilt in a caption": {path: "/todos/seed-task/edit", form: url.Values{"title": {"Seed task"}, "images": {"abc.png"}, "caption-0": {"!hi!highgh"}}, file: personal},
		"todos edit, priority rebuilt in an image":  {path: "/todos/seed-task/edit", form: url.Values{"title": {"Seed task"}, "images": {"a!hi!highgh.png"}}, file: personal},
		"goal unit":                                 {path: "/todos/add-goal", form: url.Values{"title": {"Run"}, "target": {"10"}, "unit": {"km] [planned: 2026-01-01"}}, file: personal},
		"sub-step promotion":                        {path: "/todos/seed-task/substep/promote", form: url.Values{"index": {"0"}}, file: personal},
		"idea to task":                              {path: "/ideas/plan-trip-planned-2026-01-01/to-task", form: url.Values{"target": {"personal"}}, file: personal},
		"API todo add":                              {path: "/api/v1/todos", json: `{"title":"Buy milk` + injected + `","list":"personal"}`, file: personal},
		"API todo edit":                             {path: "/api/v1/todos/seed-task", json: `{"title":"Seed task` + injected + `","list":"personal"}`, file: personal},
		"house project add":                         {path: "/house/projects/add", form: url.Values{"title": {"Paint" + injected}, "status": {"todo"}}, file: houseProjects},
		"house project edit":                        {path: "/house/projects/fix-gate/edit", form: url.Values{"title": {"Fix gate" + injected}}, file: houseProjects},
		"maintenance add":                           {path: "/house/maintenance/add", form: url.Values{"title": {"Service aircon [cadence: 1d] [tags: injected] [id: bcdfghjk]"}, "cadence": {"6m"}}, file: maintenance},
		"maintenance edit":                          {path: "/house/maintenance/clean-gutters/edit", form: url.Values{"title": {"Clean gutters [cadence: 1d] [tags: injected] [id: bcdfghjk]"}}, file: maintenance},
		"idea add":                                  {path: "/ideas/add", form: url.Values{"title": {"Kayak [status: parked] [tags: injected] [id: bcdfghjk]"}}, file: ideaFile},
		"idea edit":                                 {path: "/ideas/plan-trip-planned-2026-01-01/edit", form: url.Values{"title": {"Kayak [status: parked] [tags: injected] [id: bcdfghjk]"}}, file: ideaFile},
		"idea edit, title from body":                {path: "/ideas/plan-trip-planned-2026-01-01/edit", form: url.Values{"title": {""}, "body": {"# Kayak [status: parked] [tags: injected] [id: bcdfghjk]"}}, file: ideaFile},
		"API idea add":                              {path: "/api/v1/ideas", json: `{"title":"Kayak [status: parked] [tags: injected] [id: bcdfghjk]"}`, file: ideaFile},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var files map[file]string
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				userDir := filepath.Join(cfg.UserDataDir, "1")
				files = map[file]string{
					personal:      filepath.Join(userDir, "personal.md"),
					ideaFile:      filepath.Join(userDir, "ideas.md"),
					houseProjects: cfg.HouseProjectsPath,
					maintenance:   cfg.MaintenancePath,
				}
				seeds := map[file]string{
					personal:      "# Personal\n\n- [ ] Seed task [added: 2026-09-01]\n  - [ ] Step" + injected + "\n",
					ideaFile:      "# Ideas\n\n- [ ] Plan trip [planned: 2026-01-01] [status: untriaged] [tags: travel]\n",
					houseProjects: "# House\n\n- [ ] Fix gate [status: todo]\n",
					maintenance:   "# Maintenance\n\n- [ ] Clean gutters [cadence: 6m]\n",
				}
				if err := os.MkdirAll(userDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for f, content := range seeds {
					if err := os.WriteFile(files[f], []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			})

			var code int
			if tc.json != "" {
				req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.json))
				if strings.HasPrefix(tc.path, "/api/v1/todos/") {
					req.Method = "PUT"
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+env.token)
				rr := httptest.NewRecorder()
				env.h.ServeHTTP(rr, req)
				code = rr.Code
			} else {
				code = env.post(t, tc.path, tc.form).Code
			}
			if code >= 400 {
				t.Fatalf("status %d", code)
			}

			path := files[tc.file]
			switch tc.file {
			case personal, houseProjects:
				items, err := tracker.ParseTracker(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, it := range items {
					if it.Planned != "" || it.Priority != "" || it.HasTag("injected") || strings.Contains(it.Title, "[id:") || it.Title == "Evil" {
						t.Errorf("injected metadata on %+v", it)
					}
				}
			case maintenance:
				items, err := house.ParseMaintenance(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, it := range items {
					if it.Cadence != "6m" || slices.Contains(it.Tags, "injected") || strings.Contains(it.Title, "[id:") {
						t.Errorf("injected metadata on %+v", it)
					}
				}
			case ideaFile:
				all, err := ideas.ParseIdeas(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, idea := range all {
					if idea.Status != "untriaged" || slices.Contains(idea.Tags, "injected") || strings.Contains(idea.Title, "[id:") {
						t.Errorf("injected metadata on %+v", idea)
					}
				}
			}
		})
	}
}

// An idea's [status:] is one of the four known values; anything else reads as
// untriaged, so the status can never carry text into a CSS class.
func TestIdeaStatusAllowlist(t *testing.T) {
	cases := map[string]string{
		"untriaged":          "untriaged",
		"parked":             "parked",
		"dropped":            "dropped",
		"converted":          "converted",
		"unknown":            "untriaged",
		`x" onmouseover="y`:  "untriaged",
		"parked extra-class": "untriaged",
	}
	for status, want := range cases {
		t.Run(status, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ideas.md")
			if err := os.WriteFile(path, []byte("# Ideas\n\n- [ ] Kayak [status: "+status+"]\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			all, err := ideas.ParseIdeas(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(all) != 1 || all[0].Status != want {
				t.Fatalf("got %+v, want status %q", all, want)
			}
		})
	}
}

// A title made only of tags is empty once cleaned: every add form answers
// with its "title required" message, not a server error.
func TestTitleOfOnlyTagsIsRequired(t *testing.T) {
	cases := map[string]struct {
		path string
		form url.Values
		want string
	}{
		"todos":       {"/todos/add", url.Values{"title": {"[status: parked]"}}, "/todos?msg=title-required"},
		"goals":       {"/todos/add-goal", url.Values{"title": {"[tags: x] !high"}}, "/goals?msg=title-required"},
		"ideas":       {"/ideas/add", url.Values{"title": {"[status: parked]"}}, "/ideas?msg=title-required"},
		"house":       {"/house/projects/add", url.Values{"title": {"[tags: x]"}, "status": {"todo"}}, "/house?msg=title-required"},
		"maintenance": {"/house/maintenance/add", url.Values{"title": {"[tags: x]"}, "cadence": {"6m"}}, "/house?msg=title-required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newAppEnv(t)
			rr := env.post(t, tc.path, tc.form)
			if rr.Code != 303 || rr.Header().Get("Location") != tc.want {
				t.Errorf("got %d to %q, want 303 to %q", rr.Code, rr.Header().Get("Location"), tc.want)
			}
		})
	}
}
