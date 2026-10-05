package app

import (
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"net/url"
	"path"
	"regexp"
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

// seasonalAccentCSS is the accent the layout injects for each theme.
type seasonalAccentCSS struct{ Light, Dark string }

func buildFuncMap(loc *time.Location, authEnabled bool, version string, static func(string) (string, error), tokens theme.Tokens, reg *module.Registry) template.FuncMap {
	return template.FuncMap{
		"navItems":       reg.Nav,
		"navHasMore":     reg.HasMore,
		"navCurrent":     navCurrent,
		"navMoreCurrent": navMoreCurrent(reg),
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
		"ageBadge": func(added string) []string {
			label, level := insights.AgeBadge(added, time.Now().In(loc))
			return []string{label, level}
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
			t, err := time.Parse("2006-01-02", date)
			if err != nil {
				return date
			}
			days := int(time.Now().In(loc).Sub(t).Hours() / 24)
			switch {
			case days == 0:
				return "today"
			case days == 1:
				return "yesterday"
			case days < 7:
				return fmt.Sprintf("%d days ago", days)
			case days < 14:
				return "1 week ago"
			default:
				return fmt.Sprintf("%d weeks ago", days/7)
			}
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
		"seasonalAccent": seasonalAccentFunc(loc, tokens),
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

// templateSet is every parsed template: core pages by file name, module pages
// by "<module-id>/<file>", and the standalone login page.
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
	set.login, err = template.New("login.html").Funcs(template.FuncMap{"static": set.assets.URL}).ParseFS(web.TemplateFS, "templates/login.html")
	if err != nil {
		return set, fmt.Errorf("parsing login template: %w", err)
	}
	return set, nil
}

// parseTemplates clones the layout plus shared components for every core
// page and every file in each registered module's template directory.
func parseTemplates(fm template.FuncMap, reg *module.Registry) (core, modules map[string]*template.Template, err error) {
	base, err := template.New("layout.html").Funcs(fm).ParseFS(web.TemplateFS, "templates/layout.html", "templates/_components/*.html")
	if err != nil {
		return nil, nil, fmt.Errorf("parsing layout: %w", err)
	}

	pages := []string{"tracker.html", "goals.html", "ideas.html", "idea.html", "homepage.html", "digest.html", "calendar.html", "admin-users.html", "admin-user-form.html", "admin-password.html", "account.html", "house.html"}
	core = make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.Must(base.Clone()).ParseFS(web.TemplateFS, "templates/"+page)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		core[page] = t
	}

	modules = map[string]*template.Template{}
	for _, m := range reg.Modules() {
		id := m.Manifest().ID
		files, err := fs.Glob(web.TemplateFS, "templates/"+id+"/*.html")
		if err != nil {
			return nil, nil, err
		}
		for _, f := range files {
			t, err := template.Must(base.Clone()).ParseFS(web.TemplateFS, f)
			if err != nil {
				return nil, nil, fmt.Errorf("parsing %s: %w", f, err)
			}
			modules[id+"/"+path.Base(f)] = t
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

func seasonalAccentFunc(loc *time.Location, tokens theme.Tokens) func() seasonalAccentCSS {
	return func() seasonalAccentCSS {
		acc, err := seasonal.AccentFor(time.Now().In(loc), tokens)
		if err != nil {
			// Validated for a whole year at startup, so this is unreachable;
			// the layout then keeps theme.css's fallback accent.
			slog.Error("seasonal accent", "error", err)
			return seasonalAccentCSS{}
		}
		return seasonalAccentCSS{Light: acc.Light.Hex(), Dark: acc.Dark.Hex()}
	}
}

// loadThemeTokens parses theme.css and checks that a seasonal accent exists
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
		if _, err := seasonal.AccentFor(day, tokens); err != nil {
			return nil, err
		}
	}
	return tokens, nil
}
