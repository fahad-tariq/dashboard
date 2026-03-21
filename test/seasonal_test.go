package test

import (
	"math"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/seasonal"
)

func TestAccentHueSeasonalTargets(t *testing.T) {
	tests := []struct {
		name    string
		month   time.Month
		day     int
		wantMin int
		wantMax int
	}{
		{"mid-summer (Jan)", time.January, 15, 210, 230},
		{"late-summer (Mar)", time.March, 15, 215, 235},
		{"autumn (Apr)", time.April, 15, 235, 255},
		{"deep-autumn (May)", time.May, 15, 240, 255},
		{"winter (Jul)", time.July, 15, 190, 200},
		{"deep-winter (Aug)", time.August, 15, 190, 200},
		{"spring (Oct)", time.October, 15, 170, 185},
		{"late-spring (Nov)", time.November, 15, 175, 190},
		{"early-summer (Dec)", time.December, 15, 200, 220},
	}

	for _, tt := range tests {
		now := time.Date(2026, tt.month, tt.day, 12, 0, 0, 0, time.UTC)
		got := seasonal.AccentHue(now)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("%s: AccentHue = %d, want %d-%d", tt.name, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestAccentHueSmoothInterpolation(t *testing.T) {
	// No adjacent days should differ by more than 5 degrees (smooth transition).
	prev := seasonal.AccentHue(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	for doy := 2; doy <= 365; doy++ {
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, doy-1)
		curr := seasonal.AccentHue(now)

		diff := math.Abs(float64(curr - prev))
		if diff > 180 {
			diff = 360 - diff // handle wrap-around
		}
		if diff > 5 {
			t.Errorf("day %d -> %d: jump of %v degrees (prev=%d, curr=%d)",
				doy-1, doy, diff, prev, curr)
		}
		prev = curr
	}
}

func TestAccentHueReturnsValidRange(t *testing.T) {
	for doy := 1; doy <= 366; doy++ {
		now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, doy-1) // 2024 is a leap year
		hue := seasonal.AccentHue(now)
		if hue < 0 || hue >= 360 {
			t.Errorf("day %d: hue %d out of range [0, 360)", doy, hue)
		}
	}
}
