// Package module defines the contract a feature area implements to plug into
// the dashboard: a page, nav entries and shortcuts, and optional API routes,
// watched files, search results and a home widget.
package module

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/commentary"
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
	// ID names the module: its SSE event (changed:<ID>), its home widgets and,
	// unless Templates is set, its template directory.
	ID    string
	Title string
	Nav   []NavItem
	// Prefixes are the route prefixes the module's Routes register under.
	// Routes outside them fail start-up.
	Prefixes []string
	// Templates is the directory under web/templates/ holding the module's
	// pages; empty means ID. Modules built on the same library may share one.
	Templates string
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

// APIRouter adds routes to the bearer-token API, written relative to
// /api/v1. Every route must sit under /<module-id>, so none can shadow a core
// API route; anything else fails start-up.
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
	// Priority ("high", "medium" or "low") colours the row's edge.
	Priority string
	// Severity colours the row's link.
	Severity Severity
	// Progress, if set, draws a bar under the label.
	Progress *WidgetProgress
}

// WidgetProgress is a progress bar in a widget row.
type WidgetProgress struct {
	Percent int    // 0 to 100
	Label   string // e.g. "20/100 km"
	Class   string // a progress-fill-* colour class
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
	// ID tells a module's widgets apart. The registry prefixes it with the
	// module ID, so a module's only widget can leave it empty.
	ID         string
	Title      string
	Count      int
	CountLabel string // follows the count, e.g. "open"
	Note       string // follows the count, dimmed, e.g. "12 total"
	// Items lists at most 5 rows; a Count above the rows shown adds a
	// "+N more" link.
	Items     []WidgetItem
	Link      string
	EmptyText string
	Severity  Severity
}

// HomeWidget contributes cards below the plan section, in order. An empty
// result shows nothing.
type HomeWidget interface {
	Widgets(ctx context.Context, userID int64, now time.Time) []WidgetData
}

// Deps is everything the core hands a module. Modules keep no globals.
type Deps struct {
	Location *time.Location
	Config   *config.Config
	Services *services.Registry
	Render   *Renderer
	// DataDir holds shared data files (the directory of FAMILY_PATH).
	DataDir string
	// Commentary holds AI commentary for list items.
	Commentary *commentary.Store
	// Publish tells open pages that the module's data changed. Call it after
	// every write; it sends changed:<id> once the writes settle.
	Publish func(moduleID string)
}

// Factory builds a module from its dependencies.
type Factory func(Deps) Module
