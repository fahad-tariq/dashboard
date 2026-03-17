package httputil

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxCaptionLength = 200

// SplitImageCaption splits a pipe-delimited image entry into filename and
// caption. If there is no pipe, caption is empty. Only the first pipe is used
// as a delimiter; additional pipes are part of the caption.
func SplitImageCaption(entry string) (filename, caption string) {
	before, after, found := strings.Cut(entry, "|")
	if !found {
		return strings.TrimSpace(entry), ""
	}
	return strings.TrimSpace(before), strings.TrimSpace(after)
}

// JoinImageCaption combines a filename and caption into a pipe-delimited
// string. If caption is empty, returns just the filename.
func JoinImageCaption(filename, caption string) string {
	filename = strings.TrimSpace(filename)
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return filename
	}
	return filename + "|" + caption
}

// SanitiseCaption strips forbidden characters and truncates to maxCaptionLength.
// Forbidden characters are pipes, commas, closing brackets, angle brackets, and
// double quotes -- defence in depth for code paths that might skip template
// escaping.
func SanitiseCaption(caption string) string {
	caption = strings.Map(func(r rune) rune {
		switch r {
		case '|', ',', ']', '<', '>', '"':
			return -1
		default:
			return r
		}
	}, caption)
	caption = strings.TrimSpace(caption)
	if utf8.RuneCountInString(caption) > maxCaptionLength {
		r := []rune(caption)
		caption = string(r[:maxCaptionLength])
	}
	return caption
}

// ReconstructImages zips comma-separated filenames from the "images" form
// field with per-image caption fields (caption-0, caption-1, ...). Pipes in
// the images field are stripped as a safety measure -- captions arrive only
// via the caption-N fields.
func ReconstructImages(r *http.Request) []string {
	raw := strings.ReplaceAll(r.FormValue("images"), "|", "")
	filenames := ParseCSV(raw)
	var out []string
	for i, f := range filenames {
		caption := SanitiseCaption(r.FormValue(captionFieldName(i)))
		out = append(out, JoinImageCaption(f, caption))
	}
	return out
}

// captionFieldName returns the form field name for the caption at index i.
func captionFieldName(i int) string {
	return "caption-" + strconv.Itoa(i)
}

// CutoffDate returns the date `days` ago at midnight UTC, used for
// determining whether soft-deleted items should be purged.
func CutoffDate(days int) time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days)
}

// ParseCSV splits a comma-separated string into trimmed non-empty parts.
func ParseCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	for v := range strings.SplitSeq(raw, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
