package sse

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

// Broker manages SSE client connections and broadcasts events.
type Broker struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[chan string]struct{}),
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

// ServeHTTP implements the SSE endpoint.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering.

	ch := make(chan string, 16)
	b.subscribe(ch)
	defer b.unsubscribe(ch)

	// Send initial keepalive.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

func (b *Broker) subscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[ch] = struct{}{}
	slog.Debug("sse client connected", "total", len(b.clients))
}

func (b *Broker) unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
	slog.Debug("sse client disconnected", "total", len(b.clients))
}
