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

// Every text colour must pass on each of these, in every style and theme.
var textBackgrounds = []string{"--bg", "--surface", "--surface-2"}

func TestSeasonalColourContrastEveryDayOfLeapYear(t *testing.T) {
	tokens := loadThemeTokens(t)
	for day := time.Date(2028, 1, 1, 12, 0, 0, 0, time.UTC); day.Year() == 2028; day = day.AddDate(0, 0, 1) {
		c, err := seasonal.ColourFor(day, tokens)
		if err != nil {
			t.Fatalf("%s: %v", day.Format("2006-01-02"), err)
		}
		for _, block := range theme.Blocks() {
			colour := c.Light
			if theme.ThemeOf(block) == theme.Dark {
				colour = c.Dark
			}
			for _, bg := range textBackgrounds {
				bgc := mustColour(t, tokens, block, bg)
				if r := theme.Contrast(colour, bgc); r < minTextContrast {
					t.Errorf("%s %s seasonal %s on %s: %.2f:1, want >= %.1f", day.Format("2006-01-02"), block, colour.Hex(), bg, r, minTextContrast)
				}
			}
		}
	}
}

func TestSeasonalColourKeepsHue(t *testing.T) {
	tokens := loadThemeTokens(t)
	c, err := seasonal.ColourFor(time.Date(2028, 1, 15, 12, 0, 0, 0, time.UTC), tokens)
	if err != nil {
		t.Fatal(err)
	}
	// Mid-January is summer blue: blue must dominate in both themes.
	for name, rgb := range map[string]theme.RGB{"light": c.Light, "dark": c.Dark} {
		if rgb.B <= rgb.R || rgb.B <= rgb.G {
			t.Errorf("%s seasonal colour %s is not blue", name, rgb.Hex())
		}
	}
}

func TestThemeTextTokensContrast(t *testing.T) {
	tokens := loadThemeTokens(t)
	textTokens := []string{
		"--text", "--text-muted", "--accent",
		"--success", "--warning", "--danger", "--attention",
	}
	for _, block := range theme.Blocks() {
		for _, bg := range textBackgrounds {
			bgc := mustColour(t, tokens, block, bg)
			for _, tok := range textTokens {
				fg := mustColour(t, tokens, block, tok)
				if r := theme.Contrast(fg, bgc); r < minTextContrast {
					t.Errorf("%s %s %s on %s: %.2f:1, want >= %.1f", block, tok, fg.Hex(), bg, r, minTextContrast)
				}
			}
		}
		// Text set on a tinted fill.
		for fg, bg := range map[string]string{
			"--on-accent":  "--accent",
			"--accent":     "--accent-soft",
			"--success":    "--success-soft",
			"--warning":    "--warning-soft",
			"--danger":     "--danger-soft",
			"--text":       "--pill",
			"--text-muted": "--pill",
		} {
			f, b := mustColour(t, tokens, block, fg), mustColour(t, tokens, block, bg)
			if r := theme.Contrast(f, b); r < minTextContrast {
				t.Errorf("%s %s on %s: %.2f:1, want >= %.1f", block, fg, bg, r, minTextContrast)
			}
		}
	}
}

// TestThemeNonTextTokensContrast covers what identifies a control or a
// state without text: the tick's ring, the priority edges and the focus
// ring (--accent).
func TestThemeNonTextTokensContrast(t *testing.T) {
	tokens := loadThemeTokens(t)
	for _, block := range theme.Blocks() {
		for _, bg := range textBackgrounds {
			bgc := mustColour(t, tokens, block, bg)
			for _, tok := range []string{"--ring", "--edge-high", "--edge-medium", "--accent"} {
				c := mustColour(t, tokens, block, tok)
				if r := theme.Contrast(c, bgc); r < minNonTextContrast {
					t.Errorf("%s %s %s on %s: %.2f:1, want >= %.1f", block, tok, c.Hex(), bg, r, minNonTextContrast)
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
// colour: against its own background when it sets one it can resolve,
// otherwise against every page background, in every style and theme.
func TestThemeRuleTextContrast(t *testing.T) {
	css := cssCommentRe.ReplaceAllString(loadThemeCSS(t), "")
	tokens := loadThemeTokens(t)
	checked := 0
	for _, m := range cssRuleRe.FindAllStringSubmatch(css, -1) {
		selector := strings.Join(strings.Fields(m[1]), " ")
		if theme.IsTokenBlock(selector) {
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
		var bgVar string
		if bm := cssBgRe.FindStringSubmatch(m[2]); bm != nil {
			if v := cssVarRe.FindStringSubmatch(strings.TrimSpace(bm[1])); v != nil {
				bgVar = v[1]
			}
		}
		for _, block := range theme.Blocks() {
			fg := mustColour(t, tokens, block, fgVar[1])
			bgs := textBackgrounds
			if bgVar != "" {
				if _, err := tokens.Colour(block, bgVar); err == nil {
					bgs = []string{bgVar}
				}
			}
			for _, bg := range bgs {
				bgc := mustColour(t, tokens, block, bg)
				checked++
				if r := theme.Contrast(fg, bgc); r < minTextContrast {
					t.Errorf("%s: %s %s on %s: %.2f:1, want >= %.1f", selector, block, fgVar[1], bg, r, minTextContrast)
				}
			}
		}
	}
	if checked < 300 {
		t.Fatalf("checked only %d colour pairs; is the CSS parser still matching rules?", checked)
	}
}

// TestThemeNoOpacityDimming keeps text dimming on colour tokens. Opacity
// blends text into its background, so a token that passes contrast on its
// own fails once dimmed.
func TestThemeNoOpacityDimming(t *testing.T) {
	allowed := map[string]bool{
		".plan-item-dragging": true, // drag ghost, not read
		"0%, 100%":            true, // loading pulse keyframe
	}
	opacityRe := regexp.MustCompile(`(?:^|[;\s])opacity\s*:\s*(0?\.\d+)`)
	css := cssCommentRe.ReplaceAllString(loadThemeCSS(t), "")
	for _, m := range cssRuleRe.FindAllStringSubmatch(css, -1) {
		selector := strings.Join(strings.Fields(m[1]), " ")
		if om := opacityRe.FindStringSubmatch(m[2]); om != nil && !allowed[selector] {
			t.Errorf("%s: opacity %s dims text; use --text-muted instead", selector, om[1])
		}
	}
}

// TestThemeTextBackgroundsSetColour closes a gap in TestThemeRuleTextContrast:
// a rule that sets a background but inherits its text colour cannot be
// checked, and inherited --text-muted on a tinted fill can fail AA.
// Backgrounds that never hold text (bars, fills, separators) are exempt.
func TestThemeTextBackgroundsSetColour(t *testing.T) {
	textless := regexp.MustCompile(`progress|digest-bar|filter-sep|::before|::after|::backdrop|^[0-9]+%`)
	pageBackgrounds := map[string]bool{"--bg": true, "--surface": true, "--surface-2": true, "--card-bg": true, "--nav-bg": true}
	css := cssCommentRe.ReplaceAllString(loadThemeCSS(t), "")
	for _, m := range cssRuleRe.FindAllStringSubmatch(css, -1) {
		selector := strings.Join(strings.Fields(m[1]), " ")
		bm := cssBgRe.FindStringSubmatch(m[2])
		if bm == nil || cssColourRe.MatchString(m[2]) || textless.MatchString(selector) {
			continue
		}
		bgVar := cssVarRe.FindStringSubmatch(strings.TrimSpace(bm[1]))
		if bgVar == nil || pageBackgrounds[bgVar[1]] {
			continue
		}
		t.Errorf("%s: sets background %s but inherits its text colour; set color so contrast can be checked", selector, bgVar[1])
	}
}
