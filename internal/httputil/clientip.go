package httputil

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns the address of the client that made r, without a port.
//
// Forwarding headers are only believed when the direct peer is a trusted
// proxy, and then only the rightmost X-Forwarded-For entry: that is the one
// the proxy appended itself, while everything to its left is client-supplied.
// X-Real-IP is never used.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	addr, err := netip.ParseAddr(peer)
	if err != nil || !inPrefixes(addr.Unmap(), trusted) {
		return peer
	}

	xff := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if i := strings.LastIndexByte(xff, ','); i >= 0 {
		xff = xff[i+1:]
	}
	if fwd, err := netip.ParseAddr(strings.TrimSpace(xff)); err == nil {
		return fwd.Unmap().String()
	}
	return peer
}

func inPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
