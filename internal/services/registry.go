package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

// UserServices holds the per-user service instances.
type UserServices struct {
	Personal *tracker.Service
	Ideas    *ideas.Service
}

// Registry manages per-user service instances and the shared family/house services.
type Registry struct {
	db               *sql.DB
	userDataDir      string
	familyPath       string
	loc              *time.Location
	familySvc        *tracker.Service
	houseProjectsSvc *tracker.Service

	mu    sync.RWMutex
	cache map[int64]*UserServices
}

// NewRegistry creates a new service registry.
func NewRegistry(db *sql.DB, userDataDir, familyPath, houseProjectsPath string, loc *time.Location) *Registry {
	familyStore := tracker.NewSharedStore(db, "family")
	familySvc := tracker.NewService(familyPath, "Family", familyStore, loc)

	houseStore := tracker.NewSharedStore(db, "house")
	houseProjectsSvc := tracker.NewService(houseProjectsPath, "House", houseStore, loc)

	return &Registry{
		db:               db,
		userDataDir:      userDataDir,
		familyPath:       familyPath,
		loc:              loc,
		familySvc:        familySvc,
		houseProjectsSvc: houseProjectsSvc,
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

// EnsureUserDirs creates per-user directories and skeleton files.
// Idempotent -- safe to call multiple times.
func (r *Registry) EnsureUserDirs(userID int64) error {
	base := filepath.Join(r.userDataDir, fmt.Sprintf("%d", userID))

	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", base, err)
	}

	// Create skeleton personal.md if it does not exist.
	personalPath := filepath.Join(base, "personal.md")
	if _, err := os.Stat(personalPath); os.IsNotExist(err) {
		skeleton := "# Personal\n\n"
		if err := os.WriteFile(personalPath, []byte(skeleton), 0o644); err != nil {
			return fmt.Errorf("creating personal.md: %w", err)
		}
	}

	// Create skeleton ideas.md if it does not exist.
	ideasPath := filepath.Join(base, "ideas.md")
	if _, err := os.Stat(ideasPath); os.IsNotExist(err) {
		skeleton := "# Ideas\n\n"
		if err := os.WriteFile(ideasPath, []byte(skeleton), 0o644); err != nil {
			return fmt.Errorf("creating ideas.md: %w", err)
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
	defer r.mu.Unlock()

	if svc, ok := r.cache[userID]; ok {
		return svc
	}

	base := filepath.Join(r.userDataDir, fmt.Sprintf("%d", userID))

	personalPath := filepath.Join(base, "personal.md")
	personalStore := tracker.NewUserStore(r.db, "personal", userID)
	personalSvc := tracker.NewService(personalPath, "Personal", personalStore, r.loc)

	ideasPath := filepath.Join(base, "ideas.md")
	ideaSvc := ideas.NewService(ideasPath, r.loc)

	svc := &UserServices{
		Personal: personalSvc,
		Ideas:    ideaSvc,
	}
	r.cache[userID] = svc
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
