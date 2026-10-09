// Package itemid generates and reads the permanent IDs of list items, kept
// in the files as a final [id: xxxxxxxx] tag on each item line.
//
// An ID is 8 characters from 20 consonants (no y) and the digits. With no
// vowels it can never spell a route segment such as "add" or "bulk".
package itemid

import (
	"crypto/rand"
	"regexp"
	"strings"
)

// Alphabet is the set of characters an ID is drawn from.
const Alphabet = "bcdfghjklmnpqrstvwxz0123456789"

// Len is the length of every ID.
const Len = 8

var (
	validRe = regexp.MustCompile(`^[` + Alphabet + `]{8}$`)
	// tagRe matches a final [id: ...] tag, whatever it holds.
	tagRe = regexp.MustCompile(`^(.*?)\s*\[id:\s*([^\]]*)\]\s*$`)
)

// Valid reports whether s is a well-formed ID.
func Valid(s string) bool {
	return validRe.MatchString(s)
}

// New returns a random ID from crypto/rand.
func New() string {
	// 240 is the largest multiple of len(Alphabet) below 256: bytes at or
	// above it are skipped, so every character is equally likely.
	const limit = 256 - 256%len(Alphabet)
	out := make([]byte, 0, Len)
	buf := make([]byte, 16)
	for len(out) < Len {
		if _, err := rand.Read(buf); err != nil {
			panic("itemid: crypto/rand failed: " + err.Error())
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < Len {
				out = append(out, Alphabet[int(b)%len(Alphabet)])
			}
		}
	}
	return string(out)
}

// NewUnique returns a new ID for which taken reports false.
func NewUnique(taken func(string) bool) string {
	for {
		if id := New(); !taken(id) {
			return id
		}
	}
}

// Cut splits a final [id:] tag off an item line's text. It returns the text
// before the tag and the ID. A final tag holding anything but a valid ID is
// left in the text and reported as malformed.
func Cut(line string) (rest, id string, malformed bool) {
	m := tagRe.FindStringSubmatch(line)
	if m == nil {
		return line, "", false
	}
	if v := strings.TrimSpace(m[2]); Valid(v) {
		return m[1], v, false
	}
	return line, "", true
}

// Assign gives every item without a valid ID a new one, and a new one to
// every item repeating an earlier item's ID (the first occurrence keeps it).
// It works in place through id, which returns a pointer to an item's ID
// field, and reports whether it changed anything.
func Assign[T any](items []T, id func(*T) *string) bool {
	seen := make(map[string]bool, len(items))
	for i := range items {
		if v := *id(&items[i]); Valid(v) {
			seen[v] = true
		}
	}
	// seen holds every valid ID up front, so a new ID never takes one held
	// by a later item.
	kept := make(map[string]bool, len(items))
	changed := false
	for i := range items {
		p := id(&items[i])
		if Valid(*p) && !kept[*p] {
			kept[*p] = true
			continue
		}
		*p = NewUnique(func(s string) bool { return seen[s] })
		seen[*p] = true
		kept[*p] = true
		changed = true
	}
	return changed
}
