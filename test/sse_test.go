package test

import (
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/sse"
)

func TestSSESubscribeReceivesMessages(t *testing.T) {
	b := sse.NewBroker()
	ch := make(chan string, 16)
	b.Subscribe(ch)
	defer b.Unsubscribe(ch)

	b.Send("refresh", "hello")

	select {
	case msg := <-ch:
		want := "event: refresh\ndata: hello\n\n"
		if msg != want {
			t.Errorf("got %q, want %q", msg, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestSSEUnsubscribeStopsMessages(t *testing.T) {
	b := sse.NewBroker()
	ch := make(chan string, 16)
	b.Subscribe(ch)
	b.Unsubscribe(ch)

	b.Send("refresh", "after-unsub")

	select {
	case msg := <-ch:
		// Channel was closed by Unsubscribe, so we may receive zero value.
		if msg != "" {
			t.Errorf("expected no message after unsubscribe, got %q", msg)
		}
	default:
		// Nothing received -- correct.
	}
}

func TestSSEBroadcastMultipleSubscribers(t *testing.T) {
	b := sse.NewBroker()
	ch1 := make(chan string, 16)
	ch2 := make(chan string, 16)
	b.Subscribe(ch1)
	b.Subscribe(ch2)
	defer b.Unsubscribe(ch1)
	defer b.Unsubscribe(ch2)

	b.Send("update", "data123")

	for i, ch := range []chan string{ch1, ch2} {
		select {
		case msg := <-ch:
			want := "event: update\ndata: data123\n\n"
			if msg != want {
				t.Errorf("subscriber %d: got %q, want %q", i, msg, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestSSESlowClientDropsMessage(t *testing.T) {
	b := sse.NewBroker()
	ch := make(chan string, 1) // buffer of 1
	b.Subscribe(ch)
	defer b.Unsubscribe(ch)

	// Fill the buffer.
	b.Send("e", "first")

	// This second send should be dropped without blocking.
	done := make(chan struct{})
	go func() {
		b.Send("e", "second")
		close(done)
	}()

	select {
	case <-done:
		// Send returned without blocking -- correct.
	case <-time.After(time.Second):
		t.Fatal("Send blocked on slow client")
	}
}

// Service writes publish through a debounce so the tab that made a change
// finishes its own request before the refresh arrives, as the watcher's
// debounce used to ensure; a burst of writes becomes one event per category.
func TestSSEDebouncedPublisherCoalesces(t *testing.T) {
	b := sse.NewBroker()
	ch := make(chan string, 16)
	b.Subscribe(ch)
	defer b.Unsubscribe(ch)

	publish := b.Debounced(80 * time.Millisecond)
	publish("personal")
	publish("personal")
	publish("family")
	publish("personal")

	select {
	case msg := <-ch:
		t.Fatalf("event sent before the debounce elapsed: %q", msg)
	case <-time.After(40 * time.Millisecond):
	}
	got := map[string]int{}
	deadline := time.After(time.Second)
	for len(got) < 2 {
		select {
		case msg := <-ch:
			got[msg]++
		case <-deadline:
			t.Fatalf("timed out; got %v", got)
		}
	}
	time.Sleep(150 * time.Millisecond)
	select {
	case msg := <-ch:
		t.Fatalf("extra event %q", msg)
	default:
	}
	if got["event: file-changed\ndata: personal\n\n"] != 1 || got["event: file-changed\ndata: family\n\n"] != 1 {
		t.Errorf("events = %v, want one per category", got)
	}
}
