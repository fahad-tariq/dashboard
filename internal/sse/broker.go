package sse

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"
)

// defaultHeartbeat keeps idle streams alive through proxies and lets the
// server notice dead clients.
const defaultHeartbeat = 30 * time.Second

// Broker manages SSE client connections and broadcasts events.
type Broker struct {
	mu        sync.RWMutex
	clients   map[chan string]struct{}
	heartbeat time.Duration
	done      chan struct{}
	closeOnce sync.Once

	revMu     sync.Mutex
	revisions map[string]uint64
}

func NewBroker() *Broker {
	return NewBrokerWithHeartbeat(defaultHeartbeat)
}

// NewBrokerWithHeartbeat sends a comment line to each client every interval.
func NewBrokerWithHeartbeat(interval time.Duration) *Broker {
	return &Broker{
		clients:   make(map[chan string]struct{}),
		heartbeat: interval,
		done:      make(chan struct{}),
		revisions: make(map[string]uint64),
	}
}

// Send broadcasts a message to all connected clients.
func (b *Broker) Send(event, data string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// Drop message if client is slow.
		}
	}
}

// Close ends every open stream and refuses new ones, so graceful shutdown
// is not held up by long-lived connections. Safe to call more than once.
func (b *Broker) Close() {
	b.closeOnce.Do(func() { close(b.done) })
}

// ServeHTTP implements the SSE endpoint.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-b.done:
		http.Error(w, "Shutting down", http.StatusServiceUnavailable)
		return
	default:
	}

	// Streams outlive the server's read and write timeouts by design.
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil {
		slog.Debug("clearing SSE read deadline", "error", err)
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		slog.Debug("clearing SSE write deadline", "error", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering.

	ch := make(chan string, 16)
	b.Subscribe(ch)
	defer b.Unsubscribe(ch)

	ticker := time.NewTicker(b.heartbeat)
	defer ticker.Stop()

	if !write(w, rc, ": connected\n\n") {
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case <-ticker.C:
			if !write(w, rc, ": ping\n\n") {
				return
			}
		case msg := <-ch:
			if !write(w, rc, msg) {
				return
			}
		}
	}
}

func write(w http.ResponseWriter, rc *http.ResponseController, msg string) bool {
	if _, err := fmt.Fprint(w, msg); err != nil {
		return false
	}
	return rc.Flush() == nil
}

func (b *Broker) Subscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[ch] = struct{}{}
	slog.Debug("sse client connected", "total", len(b.clients))
}

func (b *Broker) Unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
	slog.Debug("sse client disconnected", "total", len(b.clients))
}

// DebouncedChanged returns a publisher that sends one "changed:<id>" event
// once delay has passed without another call for that module. Services
// publish their own writes through it so the tab that made a change finishes
// its own request before the refresh arrives.
//
// Each call bumps the module's revision at once, and the event data is
// "<id> <revision>". A fragment response reports the revisions after its own
// write (see Revisions), so the tab that made the change can skip the echo
// while every other tab refreshes.
func (b *Broker) DebouncedChanged(delay time.Duration) func(moduleID string) {
	send := b.debounced(delay, func(id string) {
		b.revMu.Lock()
		rev := b.revisions[id]
		b.revMu.Unlock()
		b.Send("changed:"+id, fmt.Sprintf("%s %d", id, rev))
	})
	return func(id string) {
		b.revMu.Lock()
		b.revisions[id]++
		b.revMu.Unlock()
		send(id)
	}
}

// Revisions returns a copy of each module's current change revision.
func (b *Broker) Revisions() map[string]uint64 {
	b.revMu.Lock()
	defer b.revMu.Unlock()
	return maps.Clone(b.revisions)
}

func (b *Broker) debounced(delay time.Duration, send func(key string)) func(key string) {
	var mu sync.Mutex
	timers := map[string]*time.Timer{}
	return func(key string) {
		mu.Lock()
		defer mu.Unlock()
		if t, ok := timers[key]; ok && t.Stop() {
			t.Reset(delay)
			return
		}
		var t *time.Timer
		t = time.AfterFunc(delay, func() {
			mu.Lock()
			if timers[key] == t {
				delete(timers, key)
			}
			mu.Unlock()
			send(key)
		})
		timers[key] = t
	}
}
