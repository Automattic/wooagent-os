package ask

import (
	"sync"
)

// Event is one mid-flight progress signal the /v1/ask handler emits
// while a tool-use loop is in flight. Subscribers (the UI's SSE
// connection on /v1/ask/events) render these as transient inline
// blocks between turns — the "Pricing is researching SKU-1234 — about
// a minute." affordance that keeps a ~1-minute web_search benchmark
// from looking dead in the chat.
//
// v1 only emits Kind="thinking" for slow tools (currently just
// web_search). The shape is stable but additive — new Kinds can join
// without breaking the SSE protocol.
type Event struct {
	// Kind is the event class. Currently always "thinking".
	Kind string `json:"kind"`
	// Agent is the chat-mode agent slug whose tool fired (e.g.
	// "pricing"). Lets the UI label the block correctly when more
	// than one agent's tool fires in the same chat turn.
	Agent string `json:"agent"`
	// Tool is the canonical tool name (e.g. "web_search"). The UI
	// can switch on this to render tool-specific text + icons.
	Tool string `json:"tool"`
	// Message is the daemon-supplied fallback prose. UI may override
	// based on (agent, tool) when it has richer copy; this is the
	// "if nothing else, just show this string" line.
	Message string `json:"message"`
}

// SlowTools enumerates the locally-managed tools that take long enough
// to warrant a mid-flight progress event. Anthropic-managed server
// tools (web_search_20250305) emit their name as "web_search" via
// LoopOpts.OnToolStart; their local-tool counterparts (none today)
// would also live here. Add a tool here only when it's expected to
// take >5s — fast tools generate noise, not signal.
var SlowTools = map[string]bool{
	"web_search": true,
}

// ThinkingMessageFor renders the fallback prose for a given (agent,
// tool) pair. UIs are encouraged to ignore this and render their own
// tool-specific copy, but the field is populated server-side so a
// curl-driven subscriber still sees readable text.
func ThinkingMessageFor(agent, tool string) string {
	if tool == "web_search" {
		switch agent {
		case "pricing":
			return "Pricing is researching comparable retailers — about a minute."
		default:
			return "Searching the web — this can take up to a minute."
		}
	}
	return ""
}

// Broker is the in-memory pub-sub channel-bus that ferries Events from
// /v1/ask handler OnToolStart callbacks to /v1/ask/events SSE
// subscribers. One broker per daemon process; safe for concurrent use.
//
// Lifecycle: subscribers attach + detach via Subscribe / its returned
// cancel; publishers fire-and-forget via Publish. No persistence —
// events not delivered before a subscriber's cancel are dropped (with
// one exception: a small per-thread ring buffer replays the most
// recent events to a late-arriving subscriber so the UI's "open SSE
// just before POST" race doesn't drop the first thinking event).
type Broker struct {
	mu      sync.Mutex
	threads map[string]*threadHub
}

// threadHub is the per-thread state inside the Broker. Holds the
// active subscribers and a small ring buffer of recent events.
type threadHub struct {
	mu     sync.Mutex
	subs   []chan Event
	// recent is a ring buffer of the most recent events on this
	// thread. Replayed to new subscribers so a UI that subscribes a
	// beat after the first OnToolStart fires still sees the event.
	// Sized to 4 — enough for one Pricing benchmark (1× get_product +
	// 3-4× web_search) without growing unbounded.
	recent []Event
}

// recentCap is the size of the per-thread ring buffer. Picked to cover
// a worst-case Pricing chat turn (one product read + a handful of web
// searches) without holding more than a turn's worth of history.
const recentCap = 4

// NewBroker constructs an empty broker. Cheap; one per daemon.
func NewBroker() *Broker {
	return &Broker{threads: make(map[string]*threadHub)}
}

// Subscribe registers a subscriber for one thread. Returns a read-only
// channel that delivers events until the returned cancel is called.
// Cancel is idempotent — it's safe to defer cancel() at the SSE
// handler's exit without checking whether the subscription is still
// live.
//
// Late-subscriber replay: any events still in the per-thread ring
// buffer are delivered before the channel is hooked up to future
// Publish calls. Order is preserved.
func (b *Broker) Subscribe(threadID string) (<-chan Event, func()) {
	hub := b.getOrCreateHub(threadID)
	// Buffer is 16 — enough headroom for a slow consumer to fall
	// behind by one or two events without forcing Publish to drop.
	ch := make(chan Event, 16)

	hub.mu.Lock()
	// Replay the ring buffer first so a late subscriber gets context.
	for _, e := range hub.recent {
		select {
		case ch <- e:
		default:
			// Buffer full somehow — give up on replay and let live
			// events flow.
			break
		}
	}
	hub.subs = append(hub.subs, ch)
	hub.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			hub.mu.Lock()
			for i, c := range hub.subs {
				if c == ch {
					hub.subs = append(hub.subs[:i], hub.subs[i+1:]...)
					break
				}
			}
			empty := len(hub.subs) == 0 && len(hub.recent) == 0
			hub.mu.Unlock()
			close(ch)
			// If the hub is empty + drained, garbage-collect it from
			// the broker map. (Best-effort: a concurrent Publish/
			// Subscribe may re-create it, which is fine.)
			if empty {
				b.mu.Lock()
				delete(b.threads, threadID)
				b.mu.Unlock()
			}
		})
	}
	return ch, cancel
}

// Publish delivers an event to every subscriber on the thread.
// Non-blocking: if a subscriber's buffer is full, the event is dropped
// for that subscriber (others still receive it). The event is added to
// the per-thread ring buffer regardless, so a subscriber that arrives
// a moment later still sees it during replay.
func (b *Broker) Publish(threadID string, e Event) {
	if threadID == "" {
		return // No thread → nowhere to deliver; drop silently.
	}
	hub := b.getOrCreateHub(threadID)
	hub.mu.Lock()
	defer hub.mu.Unlock()

	// Ring-buffer append. Drop oldest when full.
	if len(hub.recent) >= recentCap {
		hub.recent = append(hub.recent[1:], e)
	} else {
		hub.recent = append(hub.recent, e)
	}

	for _, c := range hub.subs {
		select {
		case c <- e:
		default:
			// Subscriber not keeping up; drop. The UI can recover by
			// reloading; this is a defensive backstop, not a normal
			// failure mode.
		}
	}
}

// getOrCreateHub returns the thread's hub, creating a new one if
// needed. Holds the broker mutex for the lookup window only.
func (b *Broker) getOrCreateHub(threadID string) *threadHub {
	b.mu.Lock()
	defer b.mu.Unlock()
	if h, ok := b.threads[threadID]; ok {
		return h
	}
	h := &threadHub{}
	b.threads[threadID] = h
	return h
}
