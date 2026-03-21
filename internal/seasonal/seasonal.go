package seasonal

import (
	"math"
	"time"
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
