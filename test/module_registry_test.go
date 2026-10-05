package test

import (
	"strings"
	"testing"

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
