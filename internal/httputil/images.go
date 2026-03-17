package httputil

import (
	"net/http"
	"strings"
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
	if len(caption) > maxCaptionLength {
		caption = caption[:maxCaptionLength]
	}
	return caption
}

// ReconstructImages zips comma-separated filenames from the "images" form
// field with per-image caption fields (caption-0, caption-1, ...). Pipes in
// the images field are stripped as a safety measure -- captions arrive only
// via the caption-N fields.
func ReconstructImages(r *http.Request) []string {
	raw := strings.ReplaceAll(r.FormValue("images"), "|", "")
	filenames := parseCSV(raw)
	var out []string
	for i, f := range filenames {
		caption := SanitiseCaption(r.FormValue(captionFieldName(i)))
		out = append(out, JoinImageCaption(f, caption))
	}
	return out
}

// captionFieldName returns the form field name for the caption at index i.
func captionFieldName(i int) string {
	return "caption-" + itoa(i)
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// parseCSV splits a comma-separated string into trimmed non-empty parts.
func parseCSV(raw string) []string {
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
