package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/module"
)

type fakeModule struct{ man module.Manifest }

func (f fakeModule) Manifest() module.Manifest { return f.man }
func (f fakeModule) Routes(chi.Router)         {}

func fake(id, prefix, shortcut string, group module.Group, order int) module.Module {
	return fakeModule{module.Manifest{
		ID:       id,
		Prefixes: []string{prefix},
		Nav:      []module.NavItem{{Label: id, Path: prefix, Order: order, Group: group, Shortcut: shortcut}},
	}}
}

var testCore = module.Core{
	Nav: []module.NavItem{
		{Label: "home", Path: "/", Group: module.Primary, Shortcut: "h"},
		{Label: "todos", Path: "/todos", Order: 10, Group: module.Primary, Shortcut: "t"},
		{Label: "digest", Path: "/digest", Order: 60, Group: module.More, Shortcut: "d"},
	},
	Prefixes: []string{"/exploration"},
}

func TestModuleRegistryRejects(t *testing.T) {
	tests := map[string]struct {
		mods    []module.Module
		wantErr string
	}{
		"duplicate ID":          {[]module.Module{fake("a", "/a", "", module.More, 0), fake("a", "/b", "", module.More, 0)}, "duplicate module ID"},
		"malformed ID":          {[]module.Module{fake("Bad_ID", "/a", "", module.More, 0)}, "must match"},
		"template dir ID":       {[]module.Module{fake("_components", "/a", "", module.More, 0)}, "must match"},
		"reserved prefix":       {[]module.Module{fake("a", "/api/a", "", module.More, 0)}, "overlaps \"/api\""},
		"prefix above reserved": {[]module.Module{fake("a", "/plan", "", module.More, 0)}, "overlaps"},
		"core nav prefix":       {[]module.Module{fake("a", "/todos/a", "", module.More, 0)}, "overlaps \"/todos\""},
		"core extra prefix":     {[]module.Module{fake("a", "/exploration", "", module.More, 0)}, "owned by core"},
		"overlapping modules":   {[]module.Module{fake("a", "/a", "", module.More, 0), fake("b", "/a/b", "", module.More, 0)}, "owned by module a"},
		"root prefix":           {[]module.Module{fake("a", "/", "", module.More, 0)}, "must start with /"},
		"trailing slash":        {[]module.Module{fake("a", "/a/", "", module.More, 0)}, "must start with /"},
		"core shortcut":         {[]module.Module{fake("a", "/a", "t", module.More, 0)}, "already used by core todos"},
		"reserved key n":        {[]module.Module{fake("a", "/a", "n", module.More, 0)}, "reserved"},
		"reserved key c":        {[]module.Module{fake("a", "/a", "c", module.More, 0)}, "reserved"},
		"duplicate module key":  {[]module.Module{fake("a", "/a", "x", module.More, 0), fake("b", "/b", "x", module.More, 0)}, "already used by module a"},
		"multi-letter key":      {[]module.Module{fake("a", "/a", "xy", module.More, 0)}, "one lowercase letter"},
		"unknown group":         {[]module.Module{fake("a", "/a", "", "side", 0)}, "want primary or more"},
		"nav outside own prefix": {[]module.Module{fakeModule{module.Manifest{
			ID: "a", Prefixes: []string{"/a"},
			Nav: []module.NavItem{{Label: "a", Path: "/todos", Group: module.More}},
		}}}, "not under the module's prefixes"},
		"too many primary links": {[]module.Module{
			fake("a", "/a", "", module.Primary, 0), fake("b", "/b", "", module.Primary, 0),
			fake("c", "/c", "", module.Primary, 0), fake("e", "/e", "", module.Primary, 0),
			fake("f", "/f", "", module.Primary, 0),
		}, "at most 6"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := module.NewRegistry(testCore, tc.mods...)
			if err == nil {
				t.Fatal("NewRegistry accepted an invalid module set")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestModuleRegistryNavOrder(t *testing.T) {
	reg, err := module.NewRegistry(testCore,
		fake("b", "/b", "x", module.More, 60),
		fake("a", "/a", "y", module.Primary, 5),
		fake("sibling", "/ab", "", module.More, 60), // /ab does not overlap /a
	)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range reg.Nav() {
		got = append(got, n.Label)
	}
	// By Order; ties keep core first, then registration order.
	want := "home a todos digest b sibling"
	if strings.Join(got, " ") != want {
		t.Errorf("nav order = %q, want %q", strings.Join(got, " "), want)
	}
	if !reg.HasMore() {
		t.Error("HasMore = false with more-group items")
	}
}

type widgetModule struct {
	fakeModule
	items []module.WidgetItem
}

func (w widgetModule) Widgets(context.Context, int64, time.Time) []module.WidgetData {
	return []module.WidgetData{{Title: "W", Count: len(w.items), Items: w.items}}
}

// The registry keeps a widget action only when it posts to a local path, and
// a meta level only when the stylesheet defines it.
func TestModuleRegistryChecksWidgetItems(t *testing.T) {
	tests := map[string]struct {
		item       module.WidgetItem
		wantAction bool
		wantLevel  string
	}{
		"local action kept":          {module.WidgetItem{Action: &module.WidgetAction{Path: "/w/do"}}, true, ""},
		"absolute URL dropped":       {module.WidgetItem{Action: &module.WidgetAction{Path: "https://evil.example/x"}}, false, ""},
		"protocol-relative dropped":  {module.WidgetItem{Action: &module.WidgetAction{Path: "//evil.example/x"}}, false, ""},
		"relative path dropped":      {module.WidgetItem{Action: &module.WidgetAction{Path: "do"}}, false, ""},
		"known meta level kept":      {module.WidgetItem{Meta: &module.WidgetMeta{Text: "x", Level: "danger"}}, false, "danger"},
		"unknown meta level cleared": {module.WidgetItem{Meta: &module.WidgetMeta{Text: "x", Level: "x\" onclick=\"y"}}, false, ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := widgetModule{fakeModule{module.Manifest{ID: "w", Prefixes: []string{"/w"}}}, []module.WidgetItem{tc.item}}
			reg, err := module.NewRegistry(testCore, m)
			if err != nil {
				t.Fatal(err)
			}
			got := reg.Widgets(t.Context(), 1, time.Now())[0].Items[0]
			if (got.Action != nil) != tc.wantAction {
				t.Errorf("action kept = %v, want %v", got.Action != nil, tc.wantAction)
			}
			if got.Meta != nil && got.Meta.Level != tc.wantLevel {
				t.Errorf("meta level = %q, want %q", got.Meta.Level, tc.wantLevel)
			}
		})
	}
}
