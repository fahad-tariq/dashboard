package watcher

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/fahad/dashboard/internal/sse"
)

const debounceInterval = 500 * time.Millisecond

// UserCallback receives file change events with user context.
// userID=0 means the change is for a shared resource (e.g. family list).
type UserCallback func(userID int64, category string)

// Watch monitors directories for file changes and broadcasts SSE events.
// dirCategories maps absolute directory paths to category names
// (e.g. {"/data/ideas": "ideas"}).
// fileCategories maps absolute file paths to category names
// (e.g. {"/data/personal.md": "personal"}).
func Watch(dirCategories, fileCategories map[string]string, broker *sse.Broker, callbacks map[string]func()) error {
	return WatchWithUserCallbacks(dirCategories, fileCategories, "", broker, callbacks, nil)
}

// WatchWithUserCallbacks monitors directories for file changes, including
// per-user directories under userDataDir. Broadcasts SSE events and calls
// both legacy callbacks (for shared resources) and user callbacks (for per-user changes).
func WatchWithUserCallbacks(dirCategories, fileCategories map[string]string, userDataDir string, broker *sse.Broker, callbacks map[string]func(), userCallback UserCallback) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for dir := range dirCategories {
		if err := addRecursive(w, dir); err != nil {
			w.Close()
			return err
		}
	}

	// Ensure parent directories of file-level categories are watched.
	for filePath := range fileCategories {
		dir := filepath.Dir(filePath)
		if err := addRecursive(w, dir); err != nil {
			slog.Warn("failed to watch file category directory", "path", dir, "error", err)
		}
	}

	// Watch the user data directory if provided.
	if userDataDir != "" {
		if err := addRecursive(w, userDataDir); err != nil {
			slog.Warn("failed to watch user data directory", "path", userDataDir, "error", err)
		}
	}

	go run(w, dirCategories, fileCategories, userDataDir, broker, callbacks, userCallback)
	return nil
}

type pendingEvent struct {
	userID   int64
	category string
}

func run(w *fsnotify.Watcher, dirCategories, fileCategories map[string]string, userDataDir string, broker *sse.Broker, callbacks map[string]func(), userCallback UserCallback) {
	defer w.Close()

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	pending := map[pendingEvent]bool{}

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}

			if event.Op == fsnotify.Chmod {
				continue
			}

			if event.Op&fsnotify.Create != 0 {
				addRecursive(w, event.Name)
			}

			userID, category := classifyEventWithUser(event.Name, dirCategories, fileCategories, userDataDir)
			if category == "" {
				continue
			}

			pending[pendingEvent{userID: userID, category: category}] = true
			timer.Reset(debounceInterval)

		case <-timer.C:
			for pe := range pending {
				slog.Info("file change detected", "type", pe.category, "user_id", pe.userID)
				broker.Send("file-changed", pe.category)
				if pe.userID == 0 {
					if cb, ok := callbacks[pe.category]; ok {
						cb()
					}
				}
				if userCallback != nil {
					userCallback(pe.userID, pe.category)
				}
			}
			pending = map[pendingEvent]bool{}

		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

// ClassifyEventWithUser determines the category and user ID from a file path.
// Exported for testing.
func ClassifyEventWithUser(path string, dirCategories, fileCategories map[string]string, userDataDir string) (int64, string) {
	return classifyEventWithUser(path, dirCategories, fileCategories, userDataDir)
}

func classifyEventWithUser(path string, dirCategories, fileCategories map[string]string, userDataDir string) (int64, string) {
	name := filepath.Base(path)

	if !strings.HasSuffix(name, ".md") {
		return 0, ""
	}

	if strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
		return 0, ""
	}

	// Check file-level categories first (most specific match).
	absPath, _ := filepath.Abs(path)
	for filePath, category := range fileCategories {
		absFile, _ := filepath.Abs(filePath)
		if absPath == absFile {
			return 0, category
		}
	}

	// Check per-user data directory.
	if userDataDir != "" {
		absUserDir, _ := filepath.Abs(userDataDir)
		if strings.HasPrefix(absPath, absUserDir+string(filepath.Separator)) {
			rel := absPath[len(absUserDir)+1:]
			// Expected format: {user_id}/...
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) >= 2 {
				uid, err := strconv.ParseInt(parts[0], 10, 64)
				if err == nil {
					subpath := parts[1]
					switch {
					case subpath == "personal.md" || strings.HasPrefix(subpath, "personal"):
						return uid, "personal"
					case strings.HasPrefix(subpath, "ideas"):
						return uid, "ideas"
					case strings.HasPrefix(subpath, "explorations"):
						return uid, "exploration"
					}
				}
			}
		}
	}

	// Fall back to directory-level categories.
	for dir, category := range dirCategories {
		absDir, _ := filepath.Abs(dir)
		if strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
			return 0, category
		}
	}

	return 0, ""
}

func addRecursive(w *fsnotify.Watcher, path string) error {
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(p)
			if strings.HasPrefix(base, ".") && p != path {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "__pycache__" {
				return filepath.SkipDir
			}
			if err := w.Add(p); err != nil {
				slog.Warn("failed to watch directory", "path", p, "error", err)
			}
		}
		return nil
	})
}
