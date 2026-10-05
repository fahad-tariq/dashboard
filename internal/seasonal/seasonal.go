package seasonal

import (
	"fmt"
	"math"
	"time"

	"github.com/fahad/dashboard/internal/theme"
)

// Southern hemisphere seasonal hue targets (HSL hue 0-360).
// Interpolated smoothly using day-of-year so transitions are gradual.
var seasonalHues = []struct {
	day int // day-of-year midpoint
	hue int // target hue
}{
	{15, 220},  // mid-January: summer blue
	{75, 220},  // mid-March: late summer, still blue
	{105, 250}, // mid-April: autumn lavender
	{135, 250}, // mid-May: deep autumn
	{166, 210}, // mid-June: transitioning to winter
	{196, 195}, // mid-July: winter teal
	{227, 195}, // mid-August: deep winter
	{258, 185}, // mid-September: early spring
	{288, 175}, // mid-October: spring green-teal
	{319, 185}, // mid-November: late spring
	{349, 210}, // mid-December: transitioning to summer
}

// AccentHue returns an HSL hue (0-360) for the current time of year.
// Southern hemisphere seasons: summer Dec-Feb, autumn Mar-May,
// winter Jun-Aug, spring Sep-Nov.
func AccentHue(now time.Time) int {
	doy := now.YearDay() // 1-366

	// Find the two surrounding waypoints and interpolate.
	n := len(seasonalHues)
	for i := range n {
		curr := seasonalHues[i]
		next := seasonalHues[(i+1)%n]

		startDay := curr.day
		endDay := next.day
		if endDay <= startDay {
			endDay += 365 // wrap around year boundary
		}

		d := doy
		if d < startDay && i == n-1 {
			d += 365 // handle wrap for last segment
		}

		if d >= startDay && d < endDay {
			span := endDay - startDay
			progress := float64(d-startDay) / float64(span)
			hue := lerp(curr.hue, next.hue, progress)
			return ((hue % 360) + 360) % 360
		}
	}

	return 220 // fallback: summer blue
}

func lerp(a, b int, t float64) int {
	return int(math.Round(float64(a) + t*float64(b-a)))
}

// MinTextContrast is the WCAG 2.2 AA ratio for normal text. The seasonal
// colour is decoration (the wordmark caret, the plan progress fill), but the
// caret is a text glyph, so it is held to the text ratio.
const MinTextContrast = 4.5

// Colour is the seasonal colour for each theme.
type Colour struct {
	Light, Dark theme.RGB
}

// look is the colour each theme aims for. Lightness is only a starting
// point: hues differ in luminance, so teal at 40% is far paler than blue.
type look struct {
	saturation, lightness, step float64
}

var looks = map[string]look{
	theme.Light: {saturation: 0.62, lightness: 0.40, step: -0.005},
	theme.Dark:  {saturation: 0.70, lightness: 0.70, step: 0.005},
}

// ColourFor returns the colour for the day's hue in each theme, moving
// lightness away from the background until it reaches MinTextContrast
// against --bg, --surface and --surface-2 in every style. One value per theme serves all
// styles, so the layout injects two colours, not six.
func ColourFor(now time.Time, tokens theme.Tokens) (Colour, error) {
	hue := float64(AccentHue(now))
	light, err := colourFor(hue, tokens, theme.Light)
	if err != nil {
		return Colour{}, err
	}
	dark, err := colourFor(hue, tokens, theme.Dark)
	if err != nil {
		return Colour{}, err
	}
	return Colour{Light: light, Dark: dark}, nil
}

// Backgrounds lists the colours the seasonal colour must stand out from in
// one theme: --bg, --surface and --surface-2 of every style.
func Backgrounds(tokens theme.Tokens, themeName string) ([]theme.RGB, error) {
	var out []theme.RGB
	for _, style := range theme.Styles {
		for _, name := range []string{"--bg", "--surface", "--surface-2"} {
			c, err := tokens.Colour(theme.Block(style, themeName), name)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func colourFor(hue float64, tokens theme.Tokens, themeName string) (theme.RGB, error) {
	against, err := Backgrounds(tokens, themeName)
	if err != nil {
		return theme.RGB{}, err
	}
	lk := looks[themeName]
	for l := lk.lightness; l >= 0 && l <= 1; l += lk.step {
		c := theme.FromHSL(hue, lk.saturation, l)
		if minContrast(c, against) >= MinTextContrast {
			return c, nil
		}
	}
	return theme.RGB{}, fmt.Errorf("seasonal: no %s colour for hue %.0f reaches %.1f:1", themeName, hue, MinTextContrast)
}

func minContrast(c theme.RGB, against []theme.RGB) float64 {
	worst := math.Inf(1)
	for _, a := range against {
		worst = math.Min(worst, theme.Contrast(c, a))
	}
	return worst
}
