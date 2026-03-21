package test

import (
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/httputil"
)

func TestRotatingFlashDeterministic(t *testing.T) {
	variants := []string{"Done.", "Nice one.", "Sorted.", "Ticked off."}
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	a := httputil.RotatingFlash("plan-completed", variants, now)
	b := httputil.RotatingFlash("plan-completed", variants, now)
	if a != b {
		t.Errorf("same key+day should return same result: %q vs %q", a, b)
	}
}

func TestRotatingFlashVariesByDay(t *testing.T) {
	variants := []string{"Done.", "Nice one.", "Sorted.", "Ticked off."}
	seen := map[string]bool{}
	for d := range 4 {
		now := time.Date(2026, 3, 17+d, 12, 0, 0, 0, time.UTC)
		seen[httputil.RotatingFlash("plan-completed", variants, now)] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected variety across 4 days, saw %d unique messages", len(seen))
	}
}

func TestRotatingFlashDifferentKeys(t *testing.T) {
	variants := []string{"A", "B", "C"}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	a := httputil.RotatingFlash("task-added", variants, now)
	b := httputil.RotatingFlash("idea-added", variants, now)
	// Different keys should (usually) produce different results due to hash.
	// Not guaranteed for every pair, but with 3 variants and different hashes
	// it's very likely. We just check they're both valid.
	if a == "" || b == "" {
		t.Errorf("expected non-empty results, got %q and %q", a, b)
	}
}

func TestRotatingFlashEmptyVariants(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	got := httputil.RotatingFlash("key", nil, now)
	if got != "" {
		t.Errorf("expected empty string for nil variants, got %q", got)
	}
}

func TestRotatingFlashWrapsAround(t *testing.T) {
	variants := []string{"A", "B"}
	seen := map[string]bool{}
	// Check across a full year -- should use both variants.
	for d := range 365 {
		now := time.Date(2026, 1, 1+d, 12, 0, 0, 0, time.UTC)
		seen[httputil.RotatingFlash("test", variants, now)] = true
	}
	if len(seen) != 2 {
		t.Errorf("expected both variants over a year, saw %d", len(seen))
	}
}
