// Package changes lets markdown-backed services publish their own writes and
// recognise them when the file watcher reports the same change back.
package changes

import (
	"crypto/sha256"
	"sync"
)

// Recorder remembers the content hash of the file as a service last wrote or
// read it. Embed it in a service: OnChange is promoted as the service's.
type Recorder struct {
	mu       sync.Mutex
	last     [sha256.Size]byte
	onChange func()
}

// OnChange sets the function called after each successful write, typically
// one that broadcasts an SSE event for the service's category.
func (r *Recorder) OnChange(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// Wrote records data as the file's current content and publishes the change.
func (r *Recorder) Wrote(data []byte) {
	r.mu.Lock()
	r.last = sha256.Sum256(data)
	fn := r.onChange
	r.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Record records data as the file's current content without publishing:
// for a service's own rewrite of a file it is loading, which open pages
// learn about from the load itself.
func (r *Recorder) Record(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = sha256.Sum256(data)
}

// Differs reports whether data differs from the last content recorded, and
// records data either way, so the same external edit is only acted on once.
func (r *Recorder) Differs(data []byte) bool {
	sum := sha256.Sum256(data)
	r.mu.Lock()
	defer r.mu.Unlock()
	if sum == r.last {
		return false
	}
	r.last = sum
	return true
}
