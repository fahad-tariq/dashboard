package httputil

import (
	"regexp"
	"strings"
)

// inlineMetaRe matches inline metadata bracket patterns like [key: value].
// Targets the specific keys used by the tracker/ideas parsers.
var inlineMetaRe = regexp.MustCompile(`\[(?:status|tags|deadline|planned|plan-order|from-idea|converted-to|deleted|added|completed|images|goal|cadence|budget|actual):\s*[^\]]*\]`)

// StripInlineMetadata removes tracker/ideas inline metadata bracket patterns
// from a string. This prevents LLM-generated content from being interpreted
// as metadata by the markdown parsers.
//
// Only used for API inputs -- web form inputs are not stripped (users may
// legitimately type bracket patterns).
func StripInlineMetadata(s string) string {
	return strings.TrimSpace(inlineMetaRe.ReplaceAllString(s, ""))
}

// validLists is the set of accepted list parameter values.
var validLists = map[string]bool{
	"personal": true,
	"todos":    true,
	"family":   true,
	"house":    true,
}

// validListsWithIdeas extends validLists for commentary which also covers ideas.
var validListsWithIdeas = map[string]bool{
	"personal": true,
	"todos":    true,
	"family":   true,
	"house":    true,
	"ideas":    true,
}

// ValidateList returns true if the list parameter is an accepted tracker list value.
func ValidateList(list string) bool {
	return validLists[list]
}

// ValidateListWithIdeas returns true if the list parameter is accepted for
// commentary (tracker lists + ideas).
func ValidateListWithIdeas(list string) bool {
	return validListsWithIdeas[list]
}

// NormaliseList maps "todos" to "personal" for consistent storage.
func NormaliseList(list string) string {
	if list == "todos" {
		return "personal"
	}
	return list
}
