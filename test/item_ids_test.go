package test

import (
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/tracker"
)

func seedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fileIDs parses path with the parser for its kind and maps each item's ID to
// its title, failing on an item without a valid ID.
func fileIDs(t *testing.T, kind, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	add := func(id, title string) {
		if !itemid.Valid(id) {
			t.Errorf("%s: item %q has no valid ID (%q)", filepath.Base(path), title, id)
		}
		if _, dup := out[id]; dup {
			t.Errorf("%s: ID %s repeated", filepath.Base(path), id)
		}
		out[id] = title
	}
	switch kind {
	case "tracker":
		items, err := tracker.ParseTracker(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			add(it.ID, it.Title)
		}
	case "ideas":
		items, err := ideas.ParseIdeas(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			add(it.ID, it.Title)
		}
	case "maintenance":
		items, err := house.ParseMaintenance(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			add(it.ID, it.Title)
		}
	}
	return out
}

const (
	trackerNoIDs = "# Personal\n\n- [ ] Pay rego [added: 2026-09-01]\n  - [ ] Find the papers\n- [ ] Pay rego [added: 2026-09-02]\n- [x] Book dentist [completed: 2026-09-03]\n"
	ideasNoIDs   = "# Ideas\n\n- [ ] Kayak [status: untriaged]\n  A body line.\n\n  More body.\n\n- [ ] Sail [status: parked]\n"
	maintNoIDs   = "# Maintenance\n\n- [ ] Clean gutters [cadence: 6m]\n  - [x] 2026-09-01 - front\n\n- [ ] Service aircon [cadence: 1y]\n"
)

// Loading a file gives every item an ID in place: the file and the cache hold
// the same IDs, duplicate titles included, and sub-steps and log entries get
// none.
func TestServicesAssignIDsOnLoad(t *testing.T) {
	cases := map[string]struct {
		kind, content string
		load          func(path string) []string // cached IDs
		wantItems     int
	}{
		"tracker": {"tracker", trackerNoIDs, func(p string) []string {
			var ids []string
			for _, it := range tracker.NewService(p, "Personal", time.UTC).All() {
				ids = append(ids, it.ID)
			}
			return ids
		}, 3},
		"ideas": {"ideas", ideasNoIDs, func(p string) []string {
			var ids []string
			for _, it := range ideas.NewService(p, time.UTC).All() {
				ids = append(ids, it.ID)
			}
			return ids
		}, 2},
		"maintenance": {"maintenance", maintNoIDs, func(p string) []string {
			items, _ := house.NewService(p, time.UTC).List()
			var ids []string
			for _, it := range items {
				ids = append(ids, it.ID)
			}
			return ids
		}, 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "list.md")
			seedFile(t, path, tc.content)
			cached := tc.load(path)
			onDisk := fileIDs(t, tc.kind, path)
			if len(onDisk) != tc.wantItems {
				t.Fatalf("%d items with IDs on disk, want %d", len(onDisk), tc.wantItems)
			}
			for _, id := range cached {
				if _, ok := onDisk[id]; !ok {
					t.Errorf("cached ID %s is not in the file", id)
				}
			}
			data, _ := os.ReadFile(path)
			if strings.Count(string(data), "[id: ") != tc.wantItems {
				t.Errorf("an indented line got an ID:\n%s", data)
			}
		})
	}
}

// A repeated ID keeps its first occurrence; later copies get new IDs.
func TestServicesRepairDuplicateIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.md")
	seedFile(t, path, "# Personal\n\n- [ ] First [id: b7k2m9xq]\n- [ ] Second [id: b7k2m9xq]\n- [ ] Third [id: t9d3fgh2]\n")
	tracker.NewService(path, "Personal", time.UTC)
	ids := fileIDs(t, "tracker", path)
	if ids["b7k2m9xq"] != "First" || ids["t9d3fgh2"] != "Third" || len(ids) != 3 {
		t.Errorf("got %v, want First to keep b7k2m9xq and Second a new ID", ids)
	}
}

// No mutation changes an existing ID: each leaves the file holding the same
// IDs (less any item it removes), and a rename keeps the item's ID.
func TestMutationsKeepIDs(t *testing.T) {
	const trackerSeed = "# Personal\n\n" +
		"- [ ] Pay rego [added: 2026-09-01] [id: b7k2m9xq]\n  - [ ] Find the papers\n" +
		"- [ ] Pay rego [added: 2026-09-02] [id: t9d3fgh2]\n" +
		"- [ ] Old [deleted: 2020-01-01] [id: 2c4v6b8n]\n"
	const ideasSeed = "# Ideas\n\n- [ ] Kayak [status: untriaged] [id: w3th3rs7]\n\n- [ ] Sail [status: parked] [id: r4st8kd2]\n"
	const maintSeed = "# Maintenance\n\n- [ ] Clean gutters [cadence: 6m] [id: g7tt3rs2]\n\n- [ ] Service aircon [cadence: 1y] [id: 4rc0nnn1]\n"
	all := func(ids ...string) []string { return ids }

	type tcase struct {
		kind, seed string
		op         func(t *testing.T, path string) error
		want       []string          // IDs left in the file
		titles     map[string]string // ID -> title after the op
	}
	trk := func(fn func(s *tracker.Service) error) func(*testing.T, string) error {
		return func(_ *testing.T, p string) error { return fn(tracker.NewService(p, "Personal", time.UTC)) }
	}
	ids := func(fn func(s *ideas.Service) error) func(*testing.T, string) error {
		return func(_ *testing.T, p string) error { return fn(ideas.NewService(p, time.UTC)) }
	}
	mnt := func(fn func(s *house.Service) error) func(*testing.T, string) error {
		return func(_ *testing.T, p string) error { return fn(house.NewService(p, time.UTC)) }
	}
	trackerIDs := all("b7k2m9xq", "t9d3fgh2", "2c4v6b8n")
	cases := map[string]tcase{
		"tracker rename": {"tracker", trackerSeed, trk(func(s *tracker.Service) error {
			return s.ApplyEdit("pay-rego", tracker.Edit{Title: "Pay the rego"})
		}), trackerIDs, map[string]string{"b7k2m9xq": "Pay the rego", "t9d3fgh2": "Pay rego"}},
		"tracker complete":    {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.Complete("pay-rego") }), trackerIDs, nil},
		"tracker trash":       {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.Delete("pay-rego") }), trackerIDs, nil},
		"tracker restore":     {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.Restore("old") }), trackerIDs, nil},
		"tracker bulk":        {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.BulkAddTag([]string{"old"}, "car") }), trackerIDs, nil},
		"tracker bulk plan":   {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.BulkSetPlanned([]string{"old"}, "2026-10-10") }), trackerIDs, nil},
		"tracker sub-step":    {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.ToggleSubStep("pay-rego", 0) }), trackerIDs, nil},
		"tracker purge one":   {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.PermanentDelete("old") }), all("b7k2m9xq", "t9d3fgh2"), nil},
		"tracker purge trash": {"tracker", trackerSeed, trk(func(s *tracker.Service) error { return s.PurgeExpired(7) }), all("b7k2m9xq", "t9d3fgh2"), nil},
		"idea rename": {"ideas", ideasSeed, ids(func(s *ideas.Service) error { return s.Edit("kayak", "Sea kayak", "", nil, nil) }),
			all("w3th3rs7", "r4st8kd2"), map[string]string{"w3th3rs7": "Sea kayak"}},
		"idea triage":     {"ideas", ideasSeed, ids(func(s *ideas.Service) error { return s.Triage("kayak", "park") }), all("w3th3rs7", "r4st8kd2"), nil},
		"idea bulk trash": {"ideas", ideasSeed, ids(func(s *ideas.Service) error { return s.BulkDelete([]string{"kayak", "sail"}) }), all("w3th3rs7", "r4st8kd2"), nil},
		"idea convert":    {"ideas", ideasSeed, ids(func(s *ideas.Service) error { return s.MarkConverted("kayak", "b7k2m9xq") }), all("w3th3rs7", "r4st8kd2"), nil},
		"idea purge":      {"ideas", ideasSeed, ids(func(s *ideas.Service) error { return s.PermanentDelete("sail") }), all("w3th3rs7"), nil},
		"maintenance log": {"maintenance", maintSeed, mnt(func(s *house.Service) error { return s.LogCompletion("clean-gutters", "done") }), all("g7tt3rs2", "4rc0nnn1"), nil},
		"maintenance rename": {"maintenance", maintSeed, mnt(func(s *house.Service) error { return s.UpdateEdit("clean-gutters", "Clear gutters", "", nil) }),
			all("g7tt3rs2", "4rc0nnn1"), map[string]string{"g7tt3rs2": "Clear gutters"}},
		"maintenance purge": {"maintenance", maintSeed, mnt(func(s *house.Service) error { return s.PermanentDelete("service-aircon") }), all("g7tt3rs2"), nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "list.md")
			seedFile(t, path, tc.seed)
			if err := tc.op(t, path); err != nil {
				t.Fatal(err)
			}
			got := fileIDs(t, tc.kind, path)
			if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, slices.Sorted(slices.Values(tc.want))) {
				t.Errorf("IDs %v, want %v", keys, tc.want)
			}
			for id, title := range tc.titles {
				if got[id] != title {
					t.Errorf("ID %s titled %q, want %q", id, got[id], title)
				}
			}
		})
	}
}

// An item added without an ID gets one and AddItem returns it; one that
// brings an ID (a move) keeps it unless the list already holds it.
func TestAddItemIDs(t *testing.T) {
	cases := map[string]struct {
		bring    string
		wantKept bool
	}{
		"new item":                 {"", false},
		"moved item keeps its ID":  {"t9d3fgh2", true},
		"ID already in the target": {"b7k2m9xq", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "list.md")
			seedFile(t, path, "# Family\n\n- [ ] Existing [id: b7k2m9xq]\n")
			svc := tracker.NewService(path, "Family", time.UTC)
			id, err := svc.AddItem(tracker.Item{ID: tc.bring, Title: "Moved", Type: tracker.TaskType})
			if err != nil {
				t.Fatal(err)
			}
			got := fileIDs(t, "tracker", path)
			if got[id] != "Moved" || got["b7k2m9xq"] != "Existing" || len(got) != 2 {
				t.Errorf("returned %s, file %v", id, got)
			}
			if (id == tc.bring) != tc.wantKept {
				t.Errorf("returned %s, brought %q, want kept = %v", id, tc.bring, tc.wantKept)
			}
		})
	}
}

// An external edit without IDs gets them on resync; a later mutation of
// another item keeps them, so the cache and the file agree with no restart.
func TestExternalEditGetsIDsOnResync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.md")
	seedFile(t, path, "# Personal\n\n- [ ] Pay rego [id: b7k2m9xq]\n")
	svc := tracker.NewService(path, "Personal", time.UTC)
	seedFile(t, path, "# Personal\n\n- [ ] Pay rego [id: b7k2m9xq]\n- [ ] Typed by hand\n")
	if changed, err := svc.ResyncIfChanged(); err != nil || !changed {
		t.Fatalf("ResyncIfChanged = %v, %v", changed, err)
	}
	var newID string
	for _, it := range svc.All() {
		if it.Title == "Typed by hand" {
			newID = it.ID
		}
	}
	if !itemid.Valid(newID) {
		t.Fatalf("resynced item has no ID in the cache: %q", newID)
	}
	if changed, _ := svc.ResyncIfChanged(); changed {
		t.Error("the service's own ID write was taken for an external edit")
	}
	if err := svc.Complete("pay-rego"); err != nil {
		t.Fatal(err)
	}
	if got := fileIDs(t, "tracker", path); got[newID] != "Typed by hand" {
		t.Errorf("file %v lacks the cached ID %s", got, newID)
	}
	cached := svc.All()
	if i := slices.IndexFunc(cached, func(it tracker.Item) bool { return it.ID == newID }); i < 0 {
		t.Errorf("cache lost ID %s after the mutation", newID)
	}
}

// A user's lists load lazily on first use and get IDs then. References older
// files hold as slugs become IDs when they name exactly one item; ambiguous
// ones stay as they were.
func TestLazyLoadAssignsIDsAndLinksReferences(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{UserDataDir: filepath.Join(dir, "users"), FamilyPath: filepath.Join(dir, "family.md"),
		HouseProjectsPath: filepath.Join(dir, "house-projects.md"), MaintenancePath: filepath.Join(dir, "maintenance.md")}
	seedFile(t, cfg.FamilyPath, "# Family\n\n- [ ] Plan the trip [from-idea: kayak]\n")
	seedFile(t, cfg.HouseProjectsPath, "# House\n\n- [ ] Fix the gate [status: todo]\n")
	seedFile(t, cfg.MaintenancePath, "# Maintenance\n\n")
	userDir := filepath.Join(cfg.UserDataDir, "1")
	seedFile(t, filepath.Join(userDir, "personal.md"), "# Personal\n\n"+
		"- [ ] Learn Rust ownership [from-idea: learn-rust]\n"+
		"- [ ] Twin [from-idea: twin]\n- [ ] Twin\n")
	seedFile(t, filepath.Join(userDir, "ideas.md"), "# Ideas\n\n"+
		"- [ ] Learn Rust [status: converted] [converted-to: learn-rust-ownership]\n\n"+
		"- [ ] Kayak [status: converted] [converted-to: plan-the-trip]\n\n"+
		"- [ ] Twin [status: converted] [converted-to: twin]\n\n"+
		"- [ ] Twin [status: untriaged]\n\n"+
		"- [ ] Gone [status: converted] [converted-to: no-such-task]\n")

	reg := services.NewRegistry(nil, cfg.UserDataDir, cfg.FamilyPath, cfg.HouseProjectsPath, cfg.MaintenancePath, time.UTC)
	u := reg.ForUser(1)

	personal := filepath.Join(userDir, "personal.md")
	ideasPath := filepath.Join(userDir, "ideas.md")
	taskIDs := fileIDs(t, "tracker", personal)
	ideaIDs := fileIDs(t, "ideas", ideasPath)
	fileIDs(t, "tracker", cfg.FamilyPath)
	byTitle := func(m map[string]string, title string) string {
		for id, ti := range m {
			if ti == title {
				return id
			}
		}
		return ""
	}

	tasks := map[string]tracker.Item{}
	for _, it := range slices.Concat(u.Personal.All(), reg.Family().All()) {
		tasks[it.Title+"|"+it.FromIdea] = it
	}
	if _, ok := tasks["Learn Rust ownership|"+byTitle(ideaIDs, "Learn Rust")]; !ok {
		t.Errorf("personal from-idea not linked: %v", tasks)
	}
	if _, ok := tasks["Plan the trip|"+byTitle(ideaIDs, "Kayak")]; !ok {
		t.Errorf("family from-idea not linked: %v", tasks)
	}
	if _, ok := tasks["Twin|twin"]; !ok {
		t.Errorf("ambiguous from-idea changed: %v", tasks)
	}

	converted := map[string]string{}
	for _, idea := range u.Ideas.All() {
		converted[idea.Title] += idea.ConvertedTo
	}
	familyIDs := fileIDs(t, "tracker", cfg.FamilyPath)
	for title, want := range map[string]string{
		"Learn Rust": byTitle(taskIDs, "Learn Rust ownership"),
		"Kayak":      byTitle(familyIDs, "Plan the trip"),
		"Twin":       "twin",
		"Gone":       "no-such-task",
	} {
		if converted[title] != want {
			t.Errorf("%s converted-to = %q, want %q", title, converted[title], want)
		}
	}
}

// Moving a task keeps its ID; converting an idea writes IDs on both sides,
// and the task's link to its idea opens the idea page.
func TestMoveAndConvertUseIDs(t *testing.T) {
	var personal, ideasPath, family string
	env := newSeededAppEnv(t, func(cfg *config.Config) {
		userDir := filepath.Join(cfg.UserDataDir, "1")
		personal, ideasPath, family = filepath.Join(userDir, "personal.md"), filepath.Join(userDir, "ideas.md"), cfg.FamilyPath
		seedFile(t, personal, "# Personal\n\n- [ ] Pay rego [id: b7k2m9xq]\n")
		seedFile(t, ideasPath, "# Ideas\n\n- [ ] Kayak [status: untriaged] [id: w3th3rs7]\n")
	})

	if rr := env.post(t, "/todos/pay-rego/move", nil); rr.Code != 303 {
		t.Fatalf("move: status %d", rr.Code)
	}
	if got := fileIDs(t, "tracker", family); got["b7k2m9xq"] != "Pay rego" {
		t.Errorf("moved task lost its ID: %v", got)
	}

	if rr := env.post(t, "/ideas/kayak/to-task", url.Values{"target": {"personal"}}); rr.Code != 303 {
		t.Fatalf("to-task: status %d", rr.Code)
	}
	items, err := tracker.ParseTracker(personal)
	if err != nil || len(items) != 1 || items[0].FromIdea != "w3th3rs7" || !itemid.Valid(items[0].ID) {
		t.Fatalf("converted task %+v (%v), want from-idea w3th3rs7 and an ID", items, err)
	}
	all, err := ideas.ParseIdeas(ideasPath)
	if err != nil || len(all) != 1 || all[0].ConvertedTo != items[0].ID {
		t.Errorf("idea %+v, want converted-to %s", all, items[0].ID)
	}

	todos := env.get(t, "/todos")
	if !strings.Contains(todos, `From <a href="/ideas/w3th3rs7">the original idea</a>`) {
		t.Error("task lacks the link to its idea")
	}
	if page := env.get(t, "/ideas/w3th3rs7"); !strings.Contains(page, "Kayak") {
		t.Error("the idea page does not open by ID")
	}
}
