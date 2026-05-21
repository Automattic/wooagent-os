package ask

import (
	"sync"
)

// DefaultMaxMessagesPerThread caps how many turns we retain in the
// in-memory store before truncating from the oldest. Each turn is at
// most a few KB of JSON; 50 turns × 4 active threads × N operators
// keeps the daemon's working set comfortable while still giving the
// model enough conversation history to behave coherently.
const DefaultMaxMessagesPerThread = 50

// ThreadStore is the in-memory per-(operator, agent) message log. It
// is NOT authoritative for the LLM call — the client sends the full
// conversation on every /v1/ask request — but it gives the daemon a
// place to journal what was said so persistence (cross-session
// history) can swap the in-memory map for a SQLite-backed store
// without changing the surrounding code shape.
//
// Concurrency: safe for parallel calls across goroutines. Each
// thread's slice is mutated under a per-instance lock; the lock is
// global to the store for simplicity, fine until per-operator
// per-agent traffic warrants finer-grained locking.
type ThreadStore struct {
	mu      sync.Mutex
	cap     int
	threads map[string][]Message
}

// NewThreadStore returns an empty store. Pass 0 to use the default
// per-thread cap of DefaultMaxMessagesPerThread.
func NewThreadStore(maxPerThread int) *ThreadStore {
	if maxPerThread <= 0 {
		maxPerThread = DefaultMaxMessagesPerThread
	}
	return &ThreadStore{
		cap:     maxPerThread,
		threads: map[string][]Message{},
	}
}

// Append records one Message at the end of the (operator, agent)
// thread. If the thread exceeds the per-thread cap after the append,
// the oldest message is dropped (no caller signal — silent FIFO).
func (s *ThreadStore) Append(operator string, agent AgentSlug, msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := threadKey(operator, agent)
	cur := s.threads[key]
	cur = append(cur, msg)
	if len(cur) > s.cap {
		// Drop from the front (oldest). Allocate a fresh slice so the
		// underlying array can be GC'd; otherwise the old head
		// references would keep the original messages alive forever.
		drop := len(cur) - s.cap
		next := make([]Message, s.cap)
		copy(next, cur[drop:])
		cur = next
	}
	s.threads[key] = cur
}

// Snapshot returns a defensive copy of the thread for one
// (operator, agent). Callers can iterate freely without holding the
// store's lock. Returns an empty slice (not nil) when the thread is
// unknown — keeps caller code simple.
func (s *ThreadStore) Snapshot(operator string, agent AgentSlug) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.threads[threadKey(operator, agent)]
	out := make([]Message, len(cur))
	copy(out, cur)
	return out
}

// Reset clears one thread's history (or the whole store if both args
// are empty strings — used by tests to start clean). Production code
// shouldn't reach for this; the store clears on daemon restart.
func (s *ThreadStore) Reset(operator string, agent AgentSlug) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if operator == "" && agent == "" {
		s.threads = map[string][]Message{}
		return
	}
	delete(s.threads, threadKey(operator, agent))
}

// threadKey is the lookup key for the in-memory map. Pipe is a safe
// separator because neither operator names (auth-token names) nor
// agent slugs contain it.
func threadKey(operator string, agent AgentSlug) string {
	return operator + "|" + string(agent)
}
