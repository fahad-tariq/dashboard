package test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestE2ESelectorsExist catches stale Playwright specs without a browser:
// every class and id a spec selects on must still appear in a template or in
// the app's JS. Parts built at run time (${...}) are ignored.
func TestE2ESelectorsExist(t *testing.T) {
	specs, err := filepath.Glob("../e2e/tests/*.ts")
	if err != nil || len(specs) == 0 {
		t.Fatalf("no e2e specs found: %v", err)
	}
	source := appSource(t)

	selectorArgs := []*regexp.Regexp{}
	for _, q := range []string{`'([^']*)'`, `"([^"]*)"`, "`([^`]*)`"} {
		selectorArgs = append(selectorArgs, regexp.MustCompile(`(?:locator|querySelector(?:All)?(?:<[^>]*>)?|closest)\(\s*`+q))
	}
	classArg := regexp.MustCompile(`toHaveClass\(\s*/([^/]+)/`)
	interpolation := regexp.MustCompile(`\$\{[^}]*\}`)
	classRe := regexp.MustCompile(`\.([a-zA-Z][\w-]*)`)
	idRe := regexp.MustCompile(`#([a-zA-Z][\w-]*)`)
	wordRe := regexp.MustCompile(`[a-zA-Z][\w-]*`)
	// Row ids embed an item ID from the seeded data.
	rowID := regexp.MustCompile(`^(item|idea|maint|plan|pick|deleted)-`)

	missing := map[string][]string{}
	check := func(token, spec string) {
		if strings.HasSuffix(token, "-") || definedIn(source, token) {
			return
		}
		missing[token] = append(missing[token], filepath.Base(spec))
	}
	checked := 0
	for _, spec := range specs {
		b, err := os.ReadFile(spec)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		for _, re := range selectorArgs {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				sel := interpolation.ReplaceAllString(m[1], "")
				for _, c := range classRe.FindAllStringSubmatch(sel, -1) {
					check(c[1], spec)
					checked++
				}
				for _, id := range idRe.FindAllStringSubmatch(sel, -1) {
					if !rowID.MatchString(id[1]) {
						check(id[1], spec)
					}
					checked++
				}
			}
		}
		for _, m := range classArg.FindAllStringSubmatch(text, -1) {
			pattern := strings.ReplaceAll(m[1], `\b`, " ")
			for _, w := range wordRe.FindAllString(pattern, -1) {
				check(w, spec)
				checked++
			}
		}
	}
	if checked < 50 {
		t.Fatalf("checked only %d selectors; is the spec parser still matching?", checked)
	}
	tokens := make([]string, 0, len(missing))
	for tok := range missing {
		tokens = append(tokens, tok)
	}
	sort.Strings(tokens)
	for _, tok := range tokens {
		t.Errorf("%q is selected in %v but no template or app JS defines it", tok, missing[tok])
	}
}

// appSource is every template plus the app's own JS (not vendored libraries).
func appSource(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.WalkDir("../web", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if (ext != ".html" && ext != ".js") || strings.Contains(path, ".min.") || strings.HasSuffix(path, "htmx-sse.js") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func definedIn(source, token string) bool {
	re := regexp.MustCompile(`(?:^|[^\w-])` + regexp.QuoteMeta(token) + `(?:[^\w-]|$)`)
	return re.MatchString(source)
}
