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
// The callbacks map dispatches by event category (e.g. "projects", "ideas", "tracker").
func Watch(dirs []string, broker *sse.Broker, callbacks map[string]func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for _, dir := range dirs {
		if err := addRecursive(w, dir); err != nil {
			w.Close()
			return err
		}
	}

	go run(w, broker, callbacks)
	return nil
}

func run(w *fsnotify.Watcher, broker *sse.Broker, callbacks map[string]func()) {
	defer w.Close()

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	var pending string

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}

			if event.Op == fsnotify.Chmod {
				continue
			}

			// Only watch new directories, not files.
			if event.Op&fsnotify.Create != 0 {
				addRecursive(w, event.Name)
			}

			category := classifyEvent(event.Name)
			if category == "" {
				continue
			}

			pending = category
			timer.Reset(debounceInterval)

		case <-timer.C:
			if pending != "" {
				slog.Info("file change detected", "type", pending)
				broker.Send("file-changed", pending)
				if cb, ok := callbacks[pending]; ok {
					cb()
				}
				pending = ""
			}

		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

// classifyEvent returns "projects", "ideas", or "" (ignore).
// Only reacts to .md file changes. Ignores .git, .db, temp files, etc.
func classifyEvent(path string) string {
	name := filepath.Base(path)

	// Only care about markdown files.
	if !strings.HasSuffix(name, ".md") {
		return ""
	}

	// Skip anything inside .git directories.
	if strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
		return ""
	}

	// Tracker file detection (by filename).
	if name == "tracker.md" {
		return "tracker"
	}

	dir := filepath.Dir(path)
	if strings.Contains(dir, "untriaged") || strings.Contains(dir, "parked") ||
		strings.Contains(dir, "dropped") || strings.Contains(dir, "research") {
		return "ideas"
	}
	return "projects"
}

func addRecursive(w *fsnotify.Watcher, path string) error {
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(p)
			// Skip hidden dirs and common noise.
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
