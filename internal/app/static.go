package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

// staticAssets serves embedded files under /static/ with content-hashed URLs.
// Only a request carrying the current hash is cached as immutable; anything
// else must revalidate, so a deploy never leaves browsers on stale JS or CSS.
type staticAssets struct {
	fsys   fs.FS
	hashes map[string]string
}

func newStaticAssets(fsys fs.FS) (*staticAssets, error) {
	a := &staticAssets{fsys: fsys, hashes: map[string]string{}}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		a.hashes[path] = hex.EncodeToString(sum[:6])
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hashing static assets: %w", err)
	}
	return a, nil
}

// URL is the "static" template function: static "theme.css" returns
// /static/theme.css?v=<hash>. An unknown name fails the render.
func (a *staticAssets) URL(name string) (string, error) {
	h, ok := a.hashes[name]
	if !ok {
		return "", fmt.Errorf("unknown static asset %q", name)
	}
	return "/static/" + name + "?v=" + h, nil
}

func (a *staticAssets) Handler() http.Handler {
	files := http.StripPrefix("/static/", http.FileServerFS(a.fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/static/")
		if h, ok := a.hashes[name]; ok && r.URL.Query().Get("v") == h {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
