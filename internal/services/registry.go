package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fahad/dashboard/internal/atomicfile"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

// UserServices holds the per-user service instances.
type UserServices struct {
	Personal *tracker.Service
	Ideas    *ideas.Service
}

// Registry manages per-user service instances and the shared family and
// house services.
type Registry struct {
	db               *sql.DB
	userDataDir      string
	familyPath       string
	loc              *time.Location
	familySvc        *tracker.Service
	houseProjectsSvc *tracker.Service
	maintenanceSvc   *house.Service

	mu        sync.RWMutex
	cache     map[int64]*UserServices
	publish   func(moduleID string)
	overrides map[int64]userPaths
}

// userPaths are the files behind one user's services.
type userPaths struct{ personal, ideas string }

// Override is a user whose files live outside USER_DATA_DIR.
type Override struct {
	UserID   int64
	Personal string
	Ideas    string
}

// Overrides lists the users set with SetUserPaths.
func (r *Registry) Overrides() []Override {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Override, 0, len(r.overrides))
	for id, p := range r.overrides {
		out = append(out, Override{UserID: id, Personal: p.personal, Ideas: p.ideas})
	}
	return out
}

// SetUserPaths makes userID's services use the given files instead of
// USER_DATA_DIR/{id}/. No-auth mode uses it to keep PERSONAL_PATH and
// IDEAS_PATH for user 1. Call it before the user's services are first used.
func (r *Registry) SetUserPaths(userID int64, personalPath, ideasPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.overrides == nil {
		r.overrides = make(map[int64]userPaths)
	}
	r.overrides[userID] = userPaths{personal: personalPath, ideas: ideasPath}
}

// pathsFor returns userID's files. Callers hold r.mu.
func (r *Registry) pathsFor(userID int64) userPaths {
	if p, ok := r.overrides[userID]; ok {
		return p
	}
	base := filepath.Join(r.userDataDir, fmt.Sprintf("%d", userID))
	return userPaths{personal: filepath.Join(base, "personal.md"), ideas: filepath.Join(base, "ideas.md")}
}

// SetPublisher makes every service, shared and per-user, call publish with
// the ID of the module that shows it after each of its own writes.
func (r *Registry) SetPublisher(publish func(moduleID string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publish = publish
	r.familySvc.OnChange(func() { publish("family") })
	r.houseProjectsSvc.OnChange(func() { publish("house") })
	r.maintenanceSvc.OnChange(func() { publish("house") })
	for _, svc := range r.cache {
		r.wireUser(svc)
	}
}

// wireUser connects a user's services to the publisher. Callers hold r.mu.
func (r *Registry) wireUser(svc *UserServices) {
	if r.publish == nil {
		return
	}
	publish := r.publish
	svc.Personal.OnChange(func() { publish("todos") })
	svc.Ideas.OnChange(func() { publish("ideas") })
}

// NewRegistry creates a new service registry.
func NewRegistry(db *sql.DB, userDataDir, familyPath, houseProjectsPath, maintenancePath string, loc *time.Location) *Registry {
	return &Registry{
		db:               db,
		userDataDir:      userDataDir,
		familyPath:       familyPath,
		loc:              loc,
		familySvc:        tracker.NewService(familyPath, "Family", loc),
		houseProjectsSvc: tracker.NewService(houseProjectsPath, "House", loc),
		maintenanceSvc:   house.NewService(maintenancePath, loc),
		cache:            make(map[int64]*UserServices),
	}
}

// Family returns the shared family service.
func (r *Registry) Family() *tracker.Service {
	return r.familySvc
}

// HouseProjects returns the shared house projects service.
func (r *Registry) HouseProjects() *tracker.Service {
	return r.houseProjectsSvc
}

// Maintenance returns the shared house maintenance service.
func (r *Registry) Maintenance() *house.Service {
	return r.maintenanceSvc
}

// EnsureUserDirs creates per-user directories and skeleton files.
// Idempotent -- safe to call multiple times.
func (r *Registry) EnsureUserDirs(userID int64) error {
	r.mu.RLock()
	paths := r.pathsFor(userID)
	r.mu.RUnlock()

	for _, f := range []struct{ path, heading string }{
		{paths.personal, "Personal"},
		{paths.ideas, "Ideas"},
	} {
		dir := filepath.Dir(f.path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
		if _, err := os.Stat(f.path); os.IsNotExist(err) {
			if err := atomicfile.Write(f.path, []byte("# "+f.heading+"\n\n"), 0o644); err != nil {
				return fmt.Errorf("creating %s: %w", filepath.Base(f.path), err)
			}
		}
	}
	return nil
}

// ForUser returns cached per-user service instances.
// Lazily creates user directories on cache miss via EnsureUserDirs.
func (r *Registry) ForUser(userID int64) *UserServices {
	r.mu.RLock()
	if svc, ok := r.cache[userID]; ok {
		r.mu.RUnlock()
		return svc
	}
	r.mu.RUnlock()

	if err := r.EnsureUserDirs(userID); err != nil {
		slog.Error("provisioning user dirs", "user_id", userID, "error", err)
	}

	r.mu.Lock()
	if svc, ok := r.cache[userID]; ok {
		r.mu.Unlock()
		return svc
	}
	paths := r.pathsFor(userID)
	svc := &UserServices{
		Personal: tracker.NewService(paths.personal, "Personal", r.loc),
		Ideas:    ideas.NewService(paths.ideas, r.loc),
	}
	r.wireUser(svc)
	r.cache[userID] = svc
	r.mu.Unlock()

	// Older slug references become IDs on the load that assigned the IDs,
	// and never later: a slug left unresolved then must not adopt a new
	// item that happens to share it. Outside r.mu: linking reads and writes
	// services, never the registry.
	if svc.Personal.AssignedOnLoad() || svc.Ideas.AssignedOnLoad() || r.familySvc.AssignedOnLoad() || r.houseProjectsSvc.AssignedOnLoad() {
		r.linkReferences(userID, svc)
	}
	return svc
}

// EvictUser removes a user from the service cache.
func (r *Registry) EvictUser(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, userID)
}

// UserDataDir returns the base directory for per-user data.
func (r *Registry) UserDataDir() string {
	return r.userDataDir
}
