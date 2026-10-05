package httputil

import "net/http"

// UndoHeader carries the path that reverses a mutation from its handler to
// the fragment middleware, which offers it on the page's toast.
const UndoHeader = "Dash-Undo"

// OfferUndo marks the response as undoable by a POST to path.
func OfferUndo(w http.ResponseWriter, path string) {
	w.Header().Set(UndoHeader, path)
}
