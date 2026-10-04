// Package theme reads colour tokens from theme.css and computes WCAG
// contrast, so the seasonal accent and the contrast tests work from the
// same values the browser uses.
package theme

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Names of the two theme blocks in theme.css.
const (
	Light = "light"
	Dark  = "dark"
)

var themeSelectors = map[string]string{
	`:root, [data-theme="light"]`: Light,
	`[data-theme="dark"]`:         Dark,
}

var (
	blockRe = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	tokenRe = regexp.MustCompile(`(--[a-z0-9-]+)\s*:\s*([^;]+);`)
	varRe   = regexp.MustCompile(`^var\((--[a-z0-9-]+)\)$`)
	hexRe   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// Tokens maps a theme name to its custom properties, as written in the CSS.
type Tokens map[string]map[string]string

// ParseTokens extracts the custom properties declared in the light and dark
// theme blocks. Other rules are ignored.
func ParseTokens(css string) (Tokens, error) {
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	out := Tokens{}
	for _, m := range blockRe.FindAllStringSubmatch(css, -1) {
		name, ok := themeSelectors[strings.Join(strings.Fields(m[1]), " ")]
		if !ok {
			continue
		}
		if out[name] == nil {
			out[name] = map[string]string{}
		}
		for _, t := range tokenRe.FindAllStringSubmatch(m[2], -1) {
			out[name][t[1]] = strings.TrimSpace(t[2])
		}
	}
	for _, name := range []string{Light, Dark} {
		if len(out[name]) == 0 {
			return nil, fmt.Errorf("theme: no %s theme block found", name)
		}
	}
	return out, nil
}

// Colour resolves a token in one theme to a colour, following var()
// references within that theme.
func (t Tokens) Colour(themeName, token string) (RGB, error) {
	vals := t[themeName]
	v, ok := vals[token]
	for range 10 {
		if !ok {
			return RGB{}, fmt.Errorf("theme: %s token %s not defined", themeName, token)
		}
		m := varRe.FindStringSubmatch(v)
		if m == nil {
			return ParseHex(v)
		}
		v, ok = vals[m[1]]
		token = m[1]
	}
	return RGB{}, fmt.Errorf("theme: %s token %s: var() chain too deep", themeName, token)
}

// RGB is an sRGB colour with 8-bit channels.
type RGB struct{ R, G, B uint8 }

// ParseHex parses a #rrggbb colour.
func ParseHex(s string) (RGB, error) {
	if !hexRe.MatchString(s) {
		return RGB{}, fmt.Errorf("theme: %q is not a #rrggbb colour", s)
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return RGB{}, fmt.Errorf("theme: parse %q: %w", s, err)
	}
	return RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}

// Hex formats the colour as #rrggbb.
func (c RGB) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// FromHSL converts hue (degrees), saturation and lightness (0-1) to RGB.
func FromHSL(h, s, l float64) RGB {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 360
	if s == 0 {
		v := round8(l)
		return RGB{v, v, v}
	}
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	return RGB{
		round8(hueToChannel(p, q, h+1.0/3)),
		round8(hueToChannel(p, q, h)),
		round8(hueToChannel(p, q, h-1.0/3)),
	}
}

func hueToChannel(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	default:
		return p
	}
}

func round8(v float64) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(1, v)) * 255))
}

// Luminance is the WCAG 2 relative luminance.
func (c RGB) Luminance() float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// Contrast is the WCAG 2 contrast ratio between two colours (1 to 21).
func Contrast(a, b RGB) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
