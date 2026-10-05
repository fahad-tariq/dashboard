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
	pages map[string]*template.Template // "<module-id>/<file>"
}

// SetPages installs the parsed module pages.
func (r *Renderer) SetPages(pages map[string]*template.Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pages = pages
}

// Page renders web/templates/<man.ID>/<file> in the layout. It adds the
// layout data (user, current path for the nav) and resolves ?msg= through
// the manifest's flash messages. data may be nil.
func (r *Renderer) Page(w http.ResponseWriter, req *http.Request, man Manifest, file string, data map[string]any) {
	r.mu.RLock()
	t := r.pages[man.ID+"/"+file]
	r.mu.RUnlock()
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
