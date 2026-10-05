// Package watcher turns external edits to data files into SSE events.
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

// Spec is one watched file. Exactly one of Path (a shared file) and UserFile
// (a file name directly inside userDataDir/{id}/) is set. After an edit
// settles, Reload re-reads the file for the user (0 for shared files) and
// reports whether it really changed, as opposed to echoing the app's own
// write; only then is Event sent with Data. A nil Reload always sends.
type Spec struct {
	Path     string
	UserFile string
	Event    string
	Data     string
	Reload   func(userID int64) bool
}

// Watch starts watching the specs' files and returns once the watches are in
// place; events are handled on a background goroutine for the life of the
// process.
func Watch(userDataDir string, specs []Spec, broker *sse.Broker) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dirs := map[string]bool{}
	for i, s := range specs {
		if s.Path != "" {
			abs, _ := filepath.Abs(s.Path)
			specs[i].Path = abs
			dirs[filepath.Dir(abs)] = true
		}
	}
	for dir := range dirs {
		if err := w.Add(dir); err != nil {
			slog.Warn("failed to watch directory", "path", dir, "error", err)
		}
	}
	absUserDir := ""
	if userDataDir != "" {
		absUserDir, _ = filepath.Abs(userDataDir)
		if err := addRecursive(w, absUserDir); err != nil {
			slog.Warn("failed to watch user data directory", "path", absUserDir, "error", err)
		}
	}
	go run(w, absUserDir, specs, broker)
	return nil
}

type pendingEvent struct {
	spec   int
	userID int64
}

func run(w *fsnotify.Watcher, userDataDir string, specs []Spec, broker *sse.Broker) {
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
			if pe, ok := handle(w, event, userDataDir, specs); ok {
				pending[pe] = true
				timer.Reset(debounceInterval)
			}

		case <-timer.C:
			for pe := range pending {
				s := specs[pe.spec]
				if s.Reload == nil || s.Reload(pe.userID) {
					slog.Info("external file change", "event", s.Event, "data", s.Data, "user_id", pe.userID)
					broker.Send(s.Event, s.Data)
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

// handle watches a newly created user directory and reports which spec, if
// any, the event belongs to.
func handle(w *fsnotify.Watcher, event fsnotify.Event, userDataDir string, specs []Spec) (pendingEvent, bool) {
	if event.Op == fsnotify.Chmod {
		return pendingEvent{}, false
	}
	if event.Op&fsnotify.Create != 0 && userDataDir != "" && strings.HasPrefix(event.Name, userDataDir+string(filepath.Separator)) {
		if err := addRecursive(w, event.Name); err != nil {
			slog.Warn("watching new path", "path", event.Name, "error", err)
		}
	}
	spec, userID, ok := Classify(event.Name, userDataDir, specs)
	return pendingEvent{spec: spec, userID: userID}, ok
}

// Classify returns the index of the spec that owns path and the user it
// belongs to (0 for shared files). Only exact file names match: a shared
// spec's absolute path, or userDataDir/{id}/{UserFile}. userDataDir must be
// absolute; specs' Paths must be absolute.
func Classify(path, userDataDir string, specs []Spec) (spec int, userID int64, ok bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, 0, false
	}
	for i, s := range specs {
		if s.Path != "" && s.Path == abs {
			return i, 0, true
		}
	}
	if userDataDir == "" {
		return 0, 0, false
	}
	rel, err := filepath.Rel(userDataDir, abs)
	if err != nil {
		return 0, 0, false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 2 {
		return 0, 0, false
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || uid <= 0 {
		return 0, 0, false
	}
	for i, s := range specs {
		if s.UserFile != "" && s.UserFile == parts[1] {
			return i, uid, true
		}
	}
	return 0, 0, false
}

func addRecursive(w *fsnotify.Watcher, path string) error {
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if base := filepath.Base(p); strings.HasPrefix(base, ".") && p != path {
				return filepath.SkipDir
			}
			if err := w.Add(p); err != nil {
				slog.Warn("failed to watch directory", "path", p, "error", err)
			}
		}
		return nil
	})
}
