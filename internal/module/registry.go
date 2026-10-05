package module

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// MaxPrimaryNav is the most links the main bar shows; the rest go behind
// "more".
const MaxPrimaryNav = 6

// ReservedPrefixes are routes the core owns. Modules may not register under
// them.
var ReservedPrefixes = []string{
	"/api", "/login", "/logout", "/admin", "/account", "/static", "/uploads",
	"/events", "/search", "/plan", "/digest", "/commentary", "/upload",
}

// ReservedKeys are pressed after "g" or on their own by core shortcuts: home,
// calendar, digest, search, help, and "n", held for quick capture.
var ReservedKeys = []string{"h", "c", "d", "/", "?", "n", "k"}

var (
	idPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	shortcutPattern = regexp.MustCompile(`^[a-z]$`)
)

// Core is what the core itself contributes to the shared namespaces.
type Core struct {
	// Nav holds the core's own nav items (home, digest, calendar, and pages
	// not yet moved into modules). Their shortcuts may use reserved keys.
	Nav []NavItem
	// Prefixes are core routes beyond ReservedPrefixes.
	Prefixes []string
}

// Registry holds the validated modules in registration order.
type Registry struct {
	modules []Module
	nav     []NavItem
}

// NewRegistry validates mods against the core and each other. It rejects
// duplicate or malformed IDs, overlapping or reserved route prefixes,
// duplicate or reserved shortcut keys and more than MaxPrimaryNav primary
// links.
func NewRegistry(core Core, mods ...Module) (*Registry, error) {
	v, err := newValidator(core)
	if err != nil {
		return nil, err
	}
	nav := slices.Clone(core.Nav)
	for _, m := range mods {
		man := m.Manifest()
		if err := v.add(man); err != nil {
			return nil, fmt.Errorf("module %s: %w", man.ID, err)
		}
		nav = append(nav, man.Nav...)
	}

	// Stable sort: equal Order keeps core first, then registration order.
	slices.SortStableFunc(nav, func(a, b NavItem) int { return cmp.Compare(a.Order, b.Order) })
	primary := 0
	for _, n := range nav {
		if n.Group == Primary {
			primary++
		}
	}
	if primary > MaxPrimaryNav {
		return nil, fmt.Errorf("%d primary nav links, at most %d allowed", primary, MaxPrimaryNav)
	}
	return &Registry{modules: mods, nav: nav}, nil
}

// validator tracks the IDs, prefixes and shortcut keys taken so far.
type validator struct {
	ids      map[string]bool
	prefixes []prefixOwner
	keys     map[string]string // key -> owner
}

type prefixOwner struct{ prefix, by string }

const reservedOwner = "core (reserved)"

func newValidator(core Core) (*validator, error) {
	v := &validator{ids: map[string]bool{}, keys: map[string]string{}}
	for _, p := range slices.Concat(ReservedPrefixes, core.Prefixes) {
		v.prefixes = append(v.prefixes, prefixOwner{p, "core"})
	}
	for _, k := range ReservedKeys {
		v.keys[k] = reservedOwner
	}
	for _, n := range core.Nav {
		if n.Path != "/" {
			v.prefixes = append(v.prefixes, prefixOwner{n.Path, "core"})
		}
		if n.Shortcut == "" {
			continue
		}
		if by, ok := v.keys[n.Shortcut]; ok && by != reservedOwner {
			return nil, fmt.Errorf("core shortcut %q used twice", n.Shortcut)
		}
		v.keys[n.Shortcut] = "core " + n.Label
	}
	return v, nil
}

func (v *validator) add(man Manifest) error {
	if !idPattern.MatchString(man.ID) {
		return fmt.Errorf("ID %q must match %s", man.ID, idPattern)
	}
	if v.ids[man.ID] {
		return fmt.Errorf("duplicate module ID %q", man.ID)
	}
	v.ids[man.ID] = true
	for _, p := range man.Prefixes {
		if err := v.addPrefix(p, "module "+man.ID); err != nil {
			return err
		}
	}
	for _, n := range man.Nav {
		if !slices.ContainsFunc(man.Prefixes, func(p string) bool { return n.Path == p || strings.HasPrefix(n.Path, p+"/") }) {
			return fmt.Errorf("nav %q path %q is not under the module's prefixes %v", n.Label, n.Path, man.Prefixes)
		}
		if err := v.addNav(n, "module "+man.ID); err != nil {
			return err
		}
	}
	return nil
}

func (v *validator) addPrefix(p, by string) error {
	if !strings.HasPrefix(p, "/") || p == "/" || strings.HasSuffix(p, "/") {
		return fmt.Errorf("prefix %q must start with / and not end with /", p)
	}
	for _, o := range v.prefixes {
		if overlaps(p, o.prefix) {
			return fmt.Errorf("prefix %q overlaps %q owned by %s", p, o.prefix, o.by)
		}
	}
	v.prefixes = append(v.prefixes, prefixOwner{p, by})
	return nil
}

func (v *validator) addNav(n NavItem, by string) error {
	if n.Group != Primary && n.Group != More {
		return fmt.Errorf("nav %q has group %q, want primary or more", n.Label, n.Group)
	}
	if n.Shortcut == "" {
		return nil
	}
	if !shortcutPattern.MatchString(n.Shortcut) {
		return fmt.Errorf("shortcut %q must be one lowercase letter", n.Shortcut)
	}
	if owner, ok := v.keys[n.Shortcut]; ok {
		return fmt.Errorf("shortcut %q already used by %s", n.Shortcut, owner)
	}
	v.keys[n.Shortcut] = by
	return nil
}

// overlaps reports whether one prefix contains the other at a path-segment
// boundary: /ideas overlaps /ideas/x but not /ideasx.
func overlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// Modules returns the modules in registration order.
func (r *Registry) Modules() []Module { return r.modules }

// Nav returns core and module nav items sorted by Order.
func (r *Registry) Nav() []NavItem { return r.nav }

// HasMore reports whether any nav item sits behind the "more" button.
func (r *Registry) HasMore() bool {
	return slices.ContainsFunc(r.nav, func(n NavItem) bool { return n.Group == More })
}

// Searchers returns the modules that answer search, in registration order.
func (r *Registry) Searchers() []Searcher {
	var out []Searcher
	for _, m := range r.modules {
		if s, ok := m.(Searcher); ok {
			out = append(out, s)
		}
	}
	return out
}

// Widgets collects home widgets in registration order.
func (r *Registry) Widgets(ctx context.Context, userID int64, now time.Time) []WidgetData {
	var out []WidgetData
	for _, m := range r.modules {
		w, ok := m.(HomeWidget)
		if !ok {
			continue
		}
		id := m.Manifest().ID
		for _, data := range w.Widgets(ctx, userID, now) {
			if data.ID == "" {
				data.ID = id
			} else {
				data.ID = id + "-" + data.ID
			}
			if len(data.Items) > 5 {
				data.Items = data.Items[:5]
			}
			out = append(out, data)
		}
	}
	return out
}

// HomeEvents are the SSE events of the modules that contribute to the
// homepage, which refreshes on any of them.
func (r *Registry) HomeEvents() []string {
	var out []string
	for _, m := range r.modules {
		if _, ok := m.(HomeWidget); ok {
			out = append(out, "changed:"+m.Manifest().ID)
		}
	}
	return out
}

// TemplateDir returns the directory under web/templates/ holding a module's
// pages.
func TemplateDir(man Manifest) string {
	return cmp.Or(man.Templates, man.ID)
}
