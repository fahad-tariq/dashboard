package module

import (
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"sync"

	"github.com/fahad/dashboard/internal/auth"
)

// Renderer executes module pages inside the shared layout. The core fills it
// once templates are parsed, after the modules exist.
type Renderer struct {
	mu    sync.RWMutex
	pages map[string]*template.Template // "<template dir>/<file>"
}

// SetPages installs the parsed module pages.
func (r *Renderer) SetPages(pages map[string]*template.Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pages = pages
}

// Lookup returns a function finding the parsed page web/templates/<dir>/<file>,
// for handlers that build their own page data; it returns nil for an unknown
// page. Pages are installed after modules are built, so the lookup happens at
// request time.
func (r *Renderer) Lookup(dir string) func(file string) *template.Template {
	return func(file string) *template.Template {
		r.mu.RLock()
		defer r.mu.RUnlock()
		return r.pages[dir+"/"+file]
	}
}

// Page renders the module's page <file> in the layout. It adds the layout
// data (user, current path for the nav) and resolves ?msg= through the
// manifest's flash messages. data may be nil.
func (r *Renderer) Page(w http.ResponseWriter, req *http.Request, man Manifest, file string, data map[string]any) {
	t := r.Lookup(TemplateDir(man))(file)
	if t == nil {
		slog.Error("module page not found", "module", man.ID, "file", file)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	full := auth.TemplateData(req)
	for k, v := range data {
		full[k] = v
	}
	if _, set := full["Title"]; !set {
		full["Title"] = man.Title
	}
	if key := req.URL.Query().Get("msg"); key != "" {
		if msg, ok := man.Flash[key]; ok {
			full["FlashMsg"] = msg
			full["FlashError"] = slices.Contains(man.FlashErrors, key)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", full); err != nil {
		slog.Error("rendering module page", "module", man.ID, "file", file, "error", err)
	}
}
