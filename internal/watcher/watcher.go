package watcher

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/fahad/dashboard/internal/sse"
)

const debounceInterval = 500 * time.Millisecond

// Watch monitors directories for file changes and broadcasts SSE events.
// dirCategories maps absolute directory paths to category names
// (e.g. {"/data/ideas": "ideas"}).
// fileCategories maps absolute file paths to category names
// (e.g. {"/data/personal.md": "personal"}).
func Watch(dirCategories, fileCategories map[string]string, broker *sse.Broker, callbacks map[string]func()) error {
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

	go run(w, dirCategories, fileCategories, broker, callbacks)
	return nil
}

func run(w *fsnotify.Watcher, dirCategories, fileCategories map[string]string, broker *sse.Broker, callbacks map[string]func()) {
	defer w.Close()

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	pending := map[string]bool{}

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

			category := classifyEvent(event.Name, dirCategories, fileCategories)
			if category == "" {
				continue
			}

			pending[category] = true
			timer.Reset(debounceInterval)

		case <-timer.C:
			for category := range pending {
				slog.Info("file change detected", "type", category)
				broker.Send("file-changed", category)
				if cb, ok := callbacks[category]; ok {
					cb()
				}
			}
			pending = map[string]bool{}

		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

func classifyEvent(path string, dirCategories, fileCategories map[string]string) string {
	name := filepath.Base(path)

	if !strings.HasSuffix(name, ".md") {
		return ""
	}

	if strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
		return ""
	}

	// Check file-level categories first (most specific match).
	absPath, _ := filepath.Abs(path)
	for filePath, category := range fileCategories {
		absFile, _ := filepath.Abs(filePath)
		if absPath == absFile {
			return category
		}
	}

	// Fall back to directory-level categories.
	for dir, category := range dirCategories {
		absDir, _ := filepath.Abs(dir)
		if strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
			return category
		}
	}

	return ""
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
