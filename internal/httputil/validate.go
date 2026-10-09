package httputil

import (
	"regexp"
	"strings"
)

// metaKeys are the inline tag names every parser family reads.
const metaKeys = `status|tags|deadline|planned|plan-order|from-idea|converted-to|deleted|added|completed|images|goal|cadence|budget|actual|project|id`

// inlineMetaRe matches inline metadata bracket patterns like [key: value].
var inlineMetaRe = regexp.MustCompile(`\[(?:` + metaKeys + `):\s*[^\]]*\]`)

// titleMetaRe also matches a tag left open at the end of a title, which
// would otherwise swallow the tags written after it.
var titleMetaRe = regexp.MustCompile(`\[\s*(?:` + metaKeys + `)\s*:[^\]]*\]?`)

// priorityRe matches the tracker's priority markers, which its parser finds
// anywhere in an item line.
var priorityRe = regexp.MustCompile(`!(?:high|medium|low)`)

// StripInlineMetadata removes inline metadata bracket patterns from a
// string. The API applies it to bodies too; bodies are indented on disk, so
// only titles could ever be parsed as tags.
func StripInlineMetadata(s string) string {
	return strings.TrimSpace(inlineMetaRe.ReplaceAllString(s, ""))
}

// CleanTitle makes text safe to write as the title on an item line: line
// breaks become spaces, and inline tags (even unclosed ones) and priority
// markers are removed, so no title can set or change an item's metadata.
// Services apply it to every title they write.
func CleanTitle(s string) string {
	s = titleMetaRe.ReplaceAllString(s, " ")
	s = priorityRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// metaValueCleaner removes what could end a tag's brackets early or start
// a new line.
var metaValueCleaner = strings.NewReplacer("[", "", "]", "", "\r", " ", "\n", " ")

// CleanMetaValue makes a value safe inside an inline tag, such as a goal's
// unit. A value read from a file never holds these characters, so writing a
// parsed value back leaves it unchanged.
func CleanMetaValue(s string) string {
	return strings.TrimSpace(metaValueCleaner.Replace(s))
}

// CleanMetaList cleans each value of a comma-separated tag (tags, images)
// and drops those left empty. A comma inside a value would split it on the
// next read, so it goes too.
func CleanMetaList(vals []string) []string {
	var out []string
	for _, v := range vals {
		if v = strings.ReplaceAll(CleanMetaValue(v), ",", ""); v != "" {
			out = append(out, v)
		}
	}
	return out
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
