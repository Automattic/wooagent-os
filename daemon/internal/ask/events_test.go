package ask

import (
	"sync"
	"testing"
	"time"
)

// TestBroker_PublishToSubscriber asserts the basic flow: subscribe,
// publish, receive.
func TestBroker_PublishToSubscriber(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe("thread-1")
	defer cancel()

	go b.Publish("thread-1", Event{Kind: "thinking", Tool: "web_search", Message: "ok"})

	select {
	case e := <-ch:
		if e.Tool != "web_search" {
			t.Errorf("tool = %q, want web_search", e.Tool)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event within 1s")
	}
}

// TestBroker_ReplaysRecentEvents asserts the ring buffer delivers
// events that were published before a subscriber arrived — the race
// the UI hits when it POSTs /v1/ask before SSE finishes its handshake.
func TestBroker_ReplaysRecentEvents(t *testing.T) {
	b := NewBroker()
	// Publish before any subscriber.
	for i := 0; i < 3; i++ {
		b.Publish("thread-1", Event{Kind: "thinking", Tool: "web_search", Message: "early"})
	}
	ch, cancel := b.Subscribe("thread-1")
	defer cancel()

	// Should see all 3 replayed events.
	for i := 0; i < 3; i++ {
		select {
		case e := <-ch:
			if e.Message != "early" {
				t.Errorf("event %d message = %q, want early", i, e.Message)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing replayed event %d", i)
		}
	}
}

// TestBroker_RingBufferEvictsOldest asserts the buffer caps at recentCap
// and discards the oldest entry once full.
func TestBroker_RingBufferEvictsOldest(t *testing.T) {
	b := NewBroker()
	// Publish recentCap+2 events; the first 2 should be evicted.
	for i := 0; i < recentCap+2; i++ {
		b.Publish("thread-1", Event{Kind: "thinking", Tool: "web_search", Message: string(rune('a' + i))})
	}
	ch, cancel := b.Subscribe("thread-1")
	defer cancel()

	got := []string{}
	timeout := time.After(500 * time.Millisecond)
loop:
	for i := 0; i < recentCap; i++ {
		select {
		case e := <-ch:
			got = append(got, e.Message)
		case <-timeout:
			break loop
		}
	}
	if len(got) != recentCap {
		t.Errorf("got %d events, want %d", len(got), recentCap)
	}
	// Oldest events (a, b) should be gone; first should be 'c'.
	if len(got) > 0 && got[0] != "c" {
		t.Errorf("first replayed event = %q, want %q (older entries should be evicted)", got[0], "c")
	}
}

// TestBroker_MultipleSubscribersFanOut asserts that two subscribers on
// the same thread both receive each published event.
func TestBroker_MultipleSubscribersFanOut(t *testing.T) {
	b := NewBroker()
	ch1, cancel1 := b.Subscribe("thread-1")
	defer cancel1()
	ch2, cancel2 := b.Subscribe("thread-1")
	defer cancel2()

	b.Publish("thread-1", Event{Kind: "thinking", Tool: "web_search"})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case <-ch:
			// OK
		case <-time.After(time.Second):
			t.Errorf("subscriber %d missed event", i)
		}
	}
}

// TestBroker_CancelStopsDelivery asserts the cancel func unhooks the
// subscriber so subsequent Publish calls don't deliver. (Channel is
// also closed so the receiver loop exits cleanly.)
func TestBroker_CancelStopsDelivery(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe("thread-1")

	cancel()
	// Channel should be closed after cancel; reading returns zero
	// value + ok=false.
	if _, ok := <-ch; ok {
		t.Errorf("expected channel closed after cancel; got open")
	}

	// Publishing after cancel must not panic.
	b.Publish("thread-1", Event{Kind: "thinking", Tool: "web_search"})
}

// TestBroker_IsolatedThreads asserts thread-A events don't leak to
// thread-B subscribers.
func TestBroker_IsolatedThreads(t *testing.T) {
	b := NewBroker()
	chA, cancelA := b.Subscribe("A")
	defer cancelA()
	chB, cancelB := b.Subscribe("B")
	defer cancelB()

	b.Publish("A", Event{Kind: "thinking", Tool: "web_search", Message: "A-event"})

	select {
	case e := <-chA:
		if e.Message != "A-event" {
			t.Errorf("A got %q, want A-event", e.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("A subscriber missed its event")
	}
	select {
	case e := <-chB:
		t.Errorf("B should not have received %+v", e)
	case <-time.After(100 * time.Millisecond):
		// OK — no event leaked across threads.
	}
}

// TestBroker_PublishWithoutSubscribersDoesntBlock asserts the
// fire-and-forget contract — Publish never blocks even if no one is
// listening. The event lands in the ring buffer for a future
// subscriber.
func TestBroker_PublishWithoutSubscribersDoesntBlock(t *testing.T) {
	b := NewBroker()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish("no-subs", Event{Kind: "thinking", Tool: "web_search"})
		}
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Publish blocked when no subscriber present")
	}
}

// TestSlowTools sanity-checks the enrolment map. If new slow tools get
// added, update this assertion alongside.
func TestSlowTools(t *testing.T) {
	if !SlowTools["web_search"] {
		t.Errorf("web_search must be in SlowTools")
	}
	if SlowTools["list_proposals"] {
		t.Errorf("list_proposals must NOT be in SlowTools")
	}
}

// TestBroker_ConcurrentPublishSafe stresses Publish across many
// goroutines; tests with -race should pass.
func TestBroker_ConcurrentPublishSafe(t *testing.T) {
	b := NewBroker()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Publish("thread-1", Event{Kind: "thinking", Tool: "web_search"})
		}()
	}
	wg.Wait()
}
