package httputil

import (
	"fmt"
	"html/template"
	"io"
)

// PageLookup finds a parsed page template by file name, or returns nil.
type PageLookup func(name string) *template.Template

// PageMap looks pages up in a fixed map.
func PageMap(m map[string]*template.Template) PageLookup {
	return func(name string) *template.Template { return m[name] }
}

// ExecutePage renders page name inside its layout.
func ExecutePage(w io.Writer, lookup PageLookup, name string, data any) error {
	t := lookup(name)
	if t == nil {
		return fmt.Errorf("page %s not found", name)
	}
	return t.ExecuteTemplate(w, "layout.html", data)
}
