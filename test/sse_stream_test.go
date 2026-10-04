package test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/sse"
)

// openStream connects to the broker over real HTTP and returns a channel of
// lines read from the stream, closed when the stream ends.
func openStream(t *testing.T, srv *httptest.Server) <-chan string {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines
}

func waitForLine(t *testing.T, lines <-chan string, want string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("stream ended before %q", want)
			}
			if strings.HasPrefix(l, want) {
				return
			}
		case <-deadline:
			t.Fatalf("no %q within %v", want, within)
		}
	}
}

func TestSSEHeartbeat(t *testing.T) {
	b := sse.NewBrokerWithHeartbeat(50 * time.Millisecond)
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)

	lines := openStream(t, srv)
	waitForLine(t, lines, ": connected", time.Second)
	waitForLine(t, lines, ": ping", time.Second)
}

// The server's WriteTimeout must not cut long-lived event streams.
func TestSSESurvivesServerWriteTimeout(t *testing.T) {
	b := sse.NewBroker()
	srv := httptest.NewUnstartedServer(b)
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Config.ReadTimeout = 100 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)

	lines := openStream(t, srv)
	waitForLine(t, lines, ": connected", time.Second)
	time.Sleep(300 * time.Millisecond)
	b.Send("file-changed", "x")
	waitForLine(t, lines, "event: file-changed", time.Second)
}

// Close ends every open stream so graceful shutdown is not held up by SSE.
func TestSSECloseEndsStreams(t *testing.T) {
	b := sse.NewBroker()
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)

	lines := openStream(t, srv)
	waitForLine(t, lines, ": connected", time.Second)
	b.Close()
	select {
	case _, ok := <-lines:
		for ok {
			_, ok = <-lines
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open 2s after Close")
	}

	// New connections after Close are refused rather than left hanging.
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status after Close = %d, want 503", resp.StatusCode)
	}
}
