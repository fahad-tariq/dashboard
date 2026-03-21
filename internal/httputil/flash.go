package httputil

import (
	"hash/fnv"
	"time"
)

// RotatingFlash selects a message variant deterministically based on the
// day-of-year and key. Messages stay consistent within a single day but
// rotate across days. The key hash ensures different message types don't
// rotate in lockstep when they have the same number of variants.
func RotatingFlash(key string, variants []string, now time.Time) string {
	if len(variants) == 0 {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(key))
	idx := (now.YearDay() + int(h.Sum32())) % len(variants)
	if idx < 0 {
		idx += len(variants)
	}
	return variants[idx]
}
