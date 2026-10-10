package app

import (
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/module"
	"github.com/fahad/dashboard/internal/seasonal"
	"github.com/fahad/dashboard/internal/theme"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/web"
)

// seasonalCSS is the seasonal colour the layout injects for each theme.
type seasonalCSS struct{ Light, Dark string }

func buildFuncMap(loc *time.Location, authEnabled bool, version string, static func(string) (string, error), tokens theme.Tokens, reg *module.Registry) template.FuncMap {
	return template.FuncMap{
		"navItems":       reg.Nav,
		"navHasMore":     reg.HasMore,
		"navCurrent":     navCurrent,
		"navMoreCurrent": navMoreCurrent(reg),
		"homeTrigger":    homeTrigger(reg),
		"static":         static,
		"authEnabled":    func() bool { return authEnabled },
		"buildVersion":   func() string { return version },
		"percentage": func(current, target float64) int {
			if target == 0 {
				return 0
			}
			p := int(current / target * 100)
			return max(0, min(p, 100))
		},
		"formatNum": func(f float64) string {
			if f == float64(int(f)) {
				return fmt.Sprintf("%d", int(f))
			}
			return fmt.Sprintf("%g", f)
		},
		"dict": templateDict,
		"subtract": func(a, b int) int {
			return a - b
		},
		"isStale":     isStaleFunc(loc),
		"filterCount": filterCountFunc(loc),
		"ageBadge": func(added string) []string {
			label, level := insights.AgeBadge(added, time.Now().In(loc))
			return []string{label, level}
		},
		"dueLabel": func(deadline string) insights.Due {
			return insights.DueLabel(deadline, time.Now().In(loc))
		},
		"progressColour": func(current, target float64, added, deadline string) string {
			return insights.ProgressColour(current, target, added, deadline, time.Now().In(loc))
		},
		"goalPace": func(current, target float64, added, deadline string) string {
			return insights.GoalPace(current, target, added, deadline, time.Now().In(loc))
		},
		"splitImageCaption": func(entry string) []string {
			file, caption := httputil.SplitImageCaption(entry)
			return []string{file, caption}
		},
		"relativeDate": func(date string) string {
			return insights.DaysAgo(date, time.Now().In(loc))
		},
		"planPercent": func(done, total int) int {
			if total == 0 {
				return 0
			}
			return min(done*100/total, 100)
		},
		"formatDateLabel": func() string {
			return time.Now().In(loc).Format("Monday, 2 January")
		},
		"seasonalColour": seasonalColourFunc(loc, tokens),
		"planDoneMessage": func() string {
			return httputil.RotatingFlash("plan-done", []string{
				"All done for the day.",
				"That's the lot.",
				"Nothing left.",
				"Clear plate.",
			}, time.Now().In(loc))
		},
		"substeps": func(body string) []tracker.SubStep {
			return tracker.ParseSubSteps(body)
		},
		"bodyText": func(body string) string {
			return tracker.BodyWithoutSubSteps(body)
		},
		"linkify": func(text string) template.HTML {
			var b strings.Builder
			last := 0
			for _, m := range urlRe.FindAllStringIndex(text, -1) {
				b.WriteString(html.EscapeString(text[last:m[0]]))
				rawURL := text[m[0]:m[1]]
				parsed, err := url.Parse(rawURL)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.Contains(rawURL, "'") {
					b.WriteString(html.EscapeString(rawURL))
				} else {
					b.WriteString(`<a href="`)
					b.WriteString(html.EscapeString(parsed.String()))
					b.WriteString(`" target="_blank" rel="noopener">`)
					b.WriteString(html.EscapeString(rawURL))
					b.WriteString(`</a>`)
				}
				last = m[1]
			}
			b.WriteString(html.EscapeString(text[last:]))
			return template.HTML(b.String()) //nolint:gosec // G203: every segment is html.EscapeString-ed above
		},
		"truncateBody": func(body string) string {
			body = strings.ReplaceAll(body, "\n", " ")
			body = strings.TrimSpace(body)
			if len(body) > 60 {
				return body[:60] + "..."
			}
			return body
		},
	}
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"` + "`" + `]+`)

// navCurrent reports whether the page at current belongs to the nav link
// path: "/" matches only itself, other links match their subtree. current is
// any because pages that do not set CurrentPath pass nil.
func navCurrent(path string, current any) bool {
	cur, _ := current.(string)
	if path == "/" {
		return cur == "/"
	}
	return cur == path || strings.HasPrefix(cur, path+"/")
}

// navMoreCurrent reports whether the current page's nav link sits behind
// "more", so the closed menu's button can show the current section.
func navMoreCurrent(reg *module.Registry) func(current any) bool {
	return func(current any) bool {
		for _, n := range reg.Nav() {
			if n.Group == module.More && navCurrent(n.Path, current) {
				return true
			}
		}
		return false
	}
}

// coreHomeEvents are the modules whose services the homepage reads itself:
// the planner's three tracker lists, and ideas for the tag summary.
var coreHomeEvents = []string{"changed:todos", "changed:family", "changed:house", "changed:ideas"}

// homeTrigger is the homepage's hx-trigger: a refresh on the event of every
// module that contributes to it, through a widget or the core's own lists.
func homeTrigger(reg *module.Registry) func() string {
	var events []string
	for _, e := range slices.Concat(coreHomeEvents, reg.HomeEvents()) {
		if !slices.Contains(events, "sse:"+e) {
			events = append(events, "sse:"+e)
		}
	}
	trigger := strings.Join(events, ", ")
	return func() string { return trigger }
}

// templateSet is every parsed template: core pages by file name, module pages
// by "<template dir>/<file>", and the standalone login page.
type templateSet struct {
	assets  *staticAssets
	core    map[string]*template.Template
	modules map[string]*template.Template
	login   *template.Template
}

// loadTemplates parses the page templates and the standalone login page.
func loadTemplates(cfg *config.Config, version string, reg *module.Registry) (templateSet, error) {
	var set templateSet
	staticSub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return set, fmt.Errorf("static assets: %w", err)
	}
	if set.assets, err = newStaticAssets(staticSub); err != nil {
		return set, err
	}
	tokens, err := loadThemeTokens(staticSub)
	if err != nil {
		return set, err
	}
	set.core, set.modules, err = parseTemplates(buildFuncMap(cfg.Location, cfg.AuthEnabled(), version, set.assets.URL, tokens, reg), reg)
	if err != nil {
		return set, fmt.Errorf("parsing templates: %w", err)
	}
	set.login, err = template.New("login.html").Funcs(template.FuncMap{"static": set.assets.URL}).ParseFS(web.TemplateFS, "templates/login.html", "templates/_components/appearance.html")
	if err != nil {
		return set, fmt.Errorf("parsing login template: %w", err)
	}
	return set, nil
}

// parseTemplates clones the layout plus shared components for every core
// page (a top-level template defining "content") and every file in each
// registered module's template directory.
func parseTemplates(fm template.FuncMap, reg *module.Registry) (core, modules map[string]*template.Template, err error) {
	base, err := template.New("layout.html").Funcs(fm).ParseFS(web.TemplateFS, "templates/layout.html", "templates/_components/*.html")
	if err != nil {
		return nil, nil, fmt.Errorf("parsing layout: %w", err)
	}
	page := func(file string) (*template.Template, error) {
		t, err := template.Must(base.Clone()).ParseFS(web.TemplateFS, file)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", file, err)
		}
		return t, nil
	}

	files, err := fs.Glob(web.TemplateFS, "templates/*.html")
	if err != nil {
		return nil, nil, err
	}
	core = map[string]*template.Template{}
	for _, f := range files {
		if path.Base(f) == "layout.html" {
			continue
		}
		t, err := page(f)
		if err != nil {
			return nil, nil, err
		}
		// Standalone templates (login, search results) define no content.
		if t.Lookup("content") != nil {
			core[path.Base(f)] = t
		}
	}

	modules = map[string]*template.Template{}
	for _, m := range reg.Modules() {
		dir := module.TemplateDir(m.Manifest())
		files, err := fs.Glob(web.TemplateFS, "templates/"+dir+"/*.html")
		if err != nil {
			return nil, nil, err
		}
		for _, f := range files {
			key := dir + "/" + path.Base(f)
			if modules[key] != nil {
				continue // a directory two modules share
			}
			if modules[key], err = page(f); err != nil {
				return nil, nil, err
			}
		}
	}
	return core, modules, nil
}

// templateDict passes several values to a sub-template:
// {{template "x" (dict "Item" . "List" "todos")}}.
func templateDict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

// staleLevels are the insights.AgeBadge levels the "stale" list filter shows.
var staleLevels = map[string]bool{"stale": true, "old": true}

// isStaleFunc returns "true" for an added date old enough for the stale
// filter, else "", for a data-stale attribute.
func isStaleFunc(loc *time.Location) func(added string) string {
	return func(added string) string {
		if _, level := insights.AgeBadge(added, time.Now().In(loc)); staleLevels[level] {
			return "true"
		}
		return ""
	}
}

// filterCountFunc counts the items a list filter would show. list is a slice
// of structs with Tags, Priority and Added fields (tracker items or ideas);
// kind is "category", "priority" or "stale", as in the filter buttons.
func filterCountFunc(loc *time.Location) func(list any, kind, value string) int {
	stale := isStaleFunc(loc)
	return func(list any, kind, value string) int {
		v := reflect.ValueOf(list)
		if v.Kind() != reflect.Slice {
			return 0
		}
		n := 0
		for i := range v.Len() {
			field := func(name string) reflect.Value { return reflect.Indirect(v.Index(i)).FieldByName(name) }
			switch kind {
			case "category":
				if f := field("Tags"); f.IsValid() {
					if tags, ok := f.Interface().([]string); ok && slices.ContainsFunc(tags, func(t string) bool { return strings.EqualFold(t, value) }) {
						n++
					}
				}
			case "priority":
				if f := field("Priority"); f.IsValid() && f.String() == value {
					n++
				}
			case "stale":
				if f := field("Added"); f.IsValid() && stale(f.String()) == value {
					n++
				}
			}
		}
		return n
	}
}

func seasonalColourFunc(loc *time.Location, tokens theme.Tokens) func() seasonalCSS {
	return func() seasonalCSS {
		c, err := seasonal.ColourFor(time.Now().In(loc), tokens)
		if err != nil {
			// Validated for a whole year at startup, so this is unreachable;
			// the layout then keeps theme.css's fallback colour.
			slog.Error("seasonal colour", "error", err)
			return seasonalCSS{}
		}
		return seasonalCSS{Light: c.Light.Hex(), Dark: c.Dark.Hex()}
	}
}

// loadThemeTokens parses theme.css and checks that a seasonal colour exists
// for every day of a leap year, so a token edit that breaks contrast fails
// at startup rather than on some later date.
func loadThemeTokens(static fs.FS) (theme.Tokens, error) {
	css, err := fs.ReadFile(static, "theme.css")
	if err != nil {
		return nil, fmt.Errorf("theme tokens: %w", err)
	}
	tokens, err := theme.ParseTokens(string(css))
	if err != nil {
		return nil, err
	}
	for day := time.Date(2028, 1, 1, 12, 0, 0, 0, time.UTC); day.Year() == 2028; day = day.AddDate(0, 0, 1) {
		if _, err := seasonal.ColourFor(day, tokens); err != nil {
			return nil, err
		}
	}
	return tokens, nil
}
