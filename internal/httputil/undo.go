package httputil

import (
	"net/http"
	"strings"
)

// UndoHeader carries the path that reverses a mutation from its handler to
// the fragment middleware, which offers it on the page's toast.
const UndoHeader = "Dash-Undo"

// UndoFromQuery returns the ?undo= restore path a plain-POST redirect carries
// for the layout's undo button, or "" unless it is a local restore route.
func UndoFromQuery(r *http.Request) string {
	p := r.URL.Query().Get("undo")
	if !IsLocalPath(p) || !strings.HasSuffix(p, "/restore") {
		return ""
	}
	return p
}

// OfferUndo marks the response as undoable by a POST to path.
func OfferUndo(w http.ResponseWriter, path string) {
	w.Header().Set(UndoHeader, path)
}
