package test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/seasonal"
	"github.com/fahad/dashboard/internal/theme"
	"github.com/fahad/dashboard/web"
)

const (
	minTextContrast    = 4.5
	minNonTextContrast = 3.0
)

func loadThemeCSS(t *testing.T) string {
	t.Helper()
	b, err := web.StaticFS.ReadFile("static/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func loadThemeTokens(t *testing.T) theme.Tokens {
	t.Helper()
	tokens, err := theme.ParseTokens(loadThemeCSS(t))
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func mustColour(t *testing.T, tokens theme.Tokens, themeName, token string) theme.RGB {
	t.Helper()
	c, err := tokens.Colour(themeName, token)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSeasonalAccentContrastEveryDayOfLeapYear(t *testing.T) {
	tokens := loadThemeTokens(t)
	for _, themeName := range []string{theme.Light, theme.Dark} {
		base := mustColour(t, tokens, themeName, "--base")
		mantle := mustColour(t, tokens, themeName, "--mantle")
		onAccent := mustColour(t, tokens, themeName, "--on-accent")
		for day := time.Date(2028, 1, 1, 12, 0, 0, 0, time.UTC); day.Year() == 2028; day = day.AddDate(0, 0, 1) {
			acc, err := seasonal.AccentFor(day, tokens)
			if err != nil {
				t.Fatalf("%s: %v", day.Format("2006-01-02"), err)
			}
			c := acc.Light
			if themeName == theme.Dark {
				c = acc.Dark
			}
			checks := map[string]struct {
				ratio, want float64
			}{
				"text on --base":       {theme.Contrast(c, base), minTextContrast},
				"text on --mantle":     {theme.Contrast(c, mantle), minTextContrast},
				"--on-accent on fill":  {theme.Contrast(onAccent, c), minTextContrast},
				"focus ring on base":   {theme.Contrast(c, base), minNonTextContrast},
				"focus ring on mantle": {theme.Contrast(c, mantle), minNonTextContrast},
			}
			for name, ch := range checks {
				if ch.ratio < ch.want {
					t.Errorf("%s %s accent %s: %s %.2f:1, want >= %.1f", day.Format("2006-01-02"), themeName, c.Hex(), name, ch.ratio, ch.want)
				}
			}
		}
	}
}

func TestSeasonalAccentKeepsHue(t *testing.T) {
	tokens := loadThemeTokens(t)
	acc, err := seasonal.AccentFor(time.Date(2028, 1, 15, 12, 0, 0, 0, time.UTC), tokens)
	if err != nil {
		t.Fatal(err)
	}
	// Mid-January is summer blue: blue must dominate in both themes.
	for name, c := range map[string]theme.RGB{"light": acc.Light, "dark": acc.Dark} {
		if c.B <= c.R || c.B <= c.G {
			t.Errorf("%s accent %s is not blue", name, c.Hex())
		}
	}
}

func TestThemeTextTokensContrast(t *testing.T) {
	tokens := loadThemeTokens(t)
	textTokens := []string{
		"--fg", "--fg-dim", "--fg-muted",
		"--success-fg", "--warning-fg", "--attention-fg", "--danger-fg", "--tag-fg",
		"--priority-high", "--priority-medium-fg", "--priority-low",
		"--accent",
	}
	for _, themeName := range []string{theme.Light, theme.Dark} {
		for _, bg := range []string{"--base", "--mantle"} {
			bgc := mustColour(t, tokens, themeName, bg)
			for _, tok := range textTokens {
				fg, err := tokens.Colour(themeName, tok)
				if err != nil {
					t.Errorf("%v", err)
					continue
				}
				if r := theme.Contrast(fg, bgc); r < minTextContrast {
					t.Errorf("%s %s %s on %s: %.2f:1, want >= %.1f", themeName, tok, fg.Hex(), bg, r, minTextContrast)
				}
			}
		}
	}
}

var (
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssRuleRe    = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	cssColourRe  = regexp.MustCompile(`(?:^|[;\s])color\s*:\s*([^;]+)`)
	cssBgRe      = regexp.MustCompile(`(?:^|[;\s])background(?:-color)?\s*:\s*([^;]+)`)
	cssVarRe     = regexp.MustCompile(`^var\((--[a-z0-9-]+)\)$`)
)

// TestThemeRuleTextContrast checks every rule in theme.css that sets a text
// colour: against its own background when it sets one, otherwise against
// --base and --mantle, in both themes. New rules that use a raw Catppuccin
// hue for text fail here.
func TestThemeRuleTextContrast(t *testing.T) {
	css := cssCommentRe.ReplaceAllString(loadThemeCSS(t), "")
	tokens := loadThemeTokens(t)
	checked := 0
	for _, m := range cssRuleRe.FindAllStringSubmatch(css, -1) {
		selector := strings.Join(strings.Fields(m[1]), " ")
		if strings.HasPrefix(selector, ":root") || strings.HasPrefix(selector, "[data-theme=") {
			continue
		}
		cm := cssColourRe.FindStringSubmatch(m[2])
		if cm == nil {
			continue
		}
		fgVar := cssVarRe.FindStringSubmatch(strings.TrimSpace(cm[1]))
		if fgVar == nil {
			continue // inherit, currentColor and the like
		}
		bgs := []string{"--base", "--mantle"}
		if bm := cssBgRe.FindStringSubmatch(m[2]); bm != nil {
			if bgVar := cssVarRe.FindStringSubmatch(strings.TrimSpace(bm[1])); bgVar != nil {
				bgs = []string{bgVar[1]}
			}
		}
		for _, themeName := range []string{theme.Light, theme.Dark} {
			fg := mustColour(t, tokens, themeName, fgVar[1])
			for _, bg := range bgs {
				bgc := mustColour(t, tokens, themeName, bg)
				checked++
				if r := theme.Contrast(fg, bgc); r < minTextContrast {
					t.Errorf("%s: %s %s on %s: %.2f:1, want >= %.1f", selector, themeName, fgVar[1], bg, r, minTextContrast)
				}
			}
		}
	}
	if checked < 100 {
		t.Fatalf("checked only %d colour pairs; is the CSS parser still matching rules?", checked)
	}
}
