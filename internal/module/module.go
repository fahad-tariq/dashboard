// Package module defines the contract a feature area implements to plug into
// the dashboard: a page, nav entries and shortcuts, and optional API routes,
// watched files, search results and a home widget.
package module

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/services"
)

// Group places a nav item in the main bar or behind the "more" button.
type Group string

const (
	Primary Group = "primary"
	More    Group = "more"
)

// NavItem is one link in the main navigation. Shortcut is the key pressed
// after "g" to go there; empty means none.
type NavItem struct {
	Label    string
	Path     string
	Order    int
	Group    Group
	Shortcut string
}

// Manifest describes a module to the core.
type Manifest struct {
	// ID names the module: its template directory (web/templates/<ID>/),
	// its SSE event (changed:<ID>) and its home widget.
	ID    string
	Title string
	Nav   []NavItem
	// Prefixes are the route prefixes the module's Routes register under.
	Prefixes []string
	// Flash maps ?msg= keys to messages; keys in FlashErrors render as errors.
	Flash       map[string]string
	FlashErrors []string
}

// Module is the one required capability.
type Module interface {
	Manifest() Manifest
	// Routes receives a sub-router that is already authenticated and
	// protected against cross-origin requests.
	Routes(r chi.Router)
}

// APIRouter adds routes to the bearer-token API under /api/v1.
type APIRouter interface {
	APIRoutes(r chi.Router)
}

// WatchSpec names one file the module reads. Exactly one of Path (a shared
// file) and UserFile (a file name inside USER_DATA_DIR/{id}/) is set.
// Reload re-reads the file for userID (0 for shared files) and reports
// whether it differed from what the module last wrote.
type WatchSpec struct {
	Path     string
	UserFile string
	Reload   func(userID int64) (bool, error)
}

// Watcher declares the files whose external edits refresh open pages.
type Watcher interface {
	Watches() []WatchSpec
}

// SearchResult is one hit in the search overlay.
type SearchResult struct {
	Title    string
	Category string
	URL      string
	Snippet  string
}

// Searcher answers the global search for one user.
type Searcher interface {
	Search(ctx context.Context, userID int64, q string) []SearchResult
}

// SearchFunc adapts a function to Searcher.
type SearchFunc func(ctx context.Context, userID int64, q string) []SearchResult

func (f SearchFunc) Search(ctx context.Context, userID int64, q string) []SearchResult {
	return f(ctx, userID, q)
}

// WidgetItem is one row of a home widget.
type WidgetItem struct {
	Label string
	URL   string
}

// Severity styles a home widget's count.
type Severity string

const (
	SeverityNone    Severity = ""
	SeverityWarning Severity = "warning"
	SeverityDanger  Severity = "danger"
)

// WidgetData is what a module shows on the homepage. It is data, not HTML;
// one core partial renders every widget.
type WidgetData struct {
	ID        string // set by the registry from the module ID
	Title     string
	Count     int
	Items     []WidgetItem // at most 5 are shown
	Link      string
	EmptyText string
	Severity  Severity
}

// HomeWidget contributes a card below the plan section. ok=false hides it.
type HomeWidget interface {
	Widget(ctx context.Context, userID int64, now time.Time) (data WidgetData, ok bool)
}

// Deps is everything the core hands a module. Modules keep no globals.
type Deps struct {
	Location *time.Location
	Config   *config.Config
	Services *services.Registry
	Render   *Renderer
	// DataDir holds shared data files (the directory of FAMILY_PATH).
	DataDir string
	// Publish tells open pages that the module's data changed. Call it after
	// every write; it sends changed:<id> once the writes settle.
	Publish func(moduleID string)
}

// Factory builds a module from its dependencies.
type Factory func(Deps) Module
