package httputil

import "strings"

// IsLocalPath reports whether p is safe to use as a same-origin redirect
// target. Browsers treat "//host" and "/\host" as protocol-relative and strip
// tabs and newlines before resolving, so any backslash or control character
// is rejected outright.
func IsLocalPath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return false
	}
	for _, c := range p {
		if c == '\\' || c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}
