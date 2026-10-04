// Package atomicfile replaces files so that a crash or error never leaves
// them truncated and concurrent readers never see a partial write.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Temp files are named ".<target>.atomic-<random>.tmp": hidden, never ending
// in .md, and specific enough for CleanStale to remove without guessing.
const (
	tempInfix  = ".atomic-"
	tempSuffix = ".tmp"
)

// Writer performs the atomic replace. Zero-value fields use the real file
// operations; tests substitute them to inject failures at each step.
type Writer struct {
	WriteData func(f *os.File, data []byte) error
	Sync      func(f *os.File) error
	Rename    func(oldpath, newpath string) error
}

// Write atomically replaces path with data using the default operations.
func Write(path string, data []byte, defaultMode fs.FileMode) error {
	return Writer{}.Write(path, data, defaultMode)
}

// Write creates a temp file beside path, writes and fsyncs it, gives it the
// target's existing mode (or defaultMode for a new file), renames it over
// path and fsyncs the directory. On any failure the temp file is removed and
// path is left as it was.
func (w Writer) Write(path string, data []byte, defaultMode fs.FileMode) (err error) {
	writeData, syncFile, rename := w.ops()

	// Write through a symlink to its target; renaming over the link itself
	// would replace it with a regular file.
	if real, evalErr := filepath.EvalSymlinks(path); evalErr == nil {
		path = real
	}

	mode := defaultMode
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}

	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, "."+base+tempInfix+"*"+tempSuffix)
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if err = writeData(f, data); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err = syncFile(f); err != nil {
		return fmt.Errorf("syncing %s: %w", tmp, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmp, err)
	}
	if err = os.Chmod(tmp, mode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", tmp, err)
	}
	if err = rename(tmp, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	// The file is replaced at this point; reporting failure would make callers
	// treat a completed write as failed. Durability of the rename is reduced.
	if syncErr := syncDir(dir); syncErr != nil {
		slog.Warn("directory sync after atomic write failed", "dir", dir, "error", syncErr)
	}
	return nil
}

// ops returns the configured operations, falling back to the real ones.
func (w Writer) ops() (
	writeData func(*os.File, []byte) error,
	syncFile func(*os.File) error,
	rename func(string, string) error,
) {
	writeData, syncFile, rename = w.WriteData, w.Sync, w.Rename
	if writeData == nil {
		writeData = func(f *os.File, b []byte) error { _, err := f.Write(b); return err }
	}
	if syncFile == nil {
		syncFile = (*os.File).Sync
	}
	if rename == nil {
		rename = os.Rename
	}
	return writeData, syncFile, rename
}

// syncDir makes the rename itself durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening %s to sync: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("syncing directory %s: %w", dir, err)
	}
	return nil
}

// CleanStale removes temp files that an interrupted Write left under root,
// recursively. Call it at startup, before any writes. The walk is confined
// to root, so a symlink swapped in mid-walk cannot redirect a removal.
func CleanStale(root string) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		// A missing or unreadable directory must not stop startup.
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("skipping directory while removing stale temp files", "path", root, "error", err)
		}
		return nil
	}
	defer r.Close()
	return fs.WalkDir(r.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				slog.Warn("skipping directory while removing stale temp files",
					"path", filepath.Join(root, path), "error", err)
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.Type().IsRegular() && strings.HasPrefix(name, ".") &&
			strings.Contains(name, tempInfix) && strings.HasSuffix(name, tempSuffix) {
			if err := r.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		return nil
	})
}
