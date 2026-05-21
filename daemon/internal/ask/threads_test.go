package ask

import (
	"strconv"
	"testing"
)

func TestThreadStore_AppendAndSnapshot(t *testing.T) {
	s := NewThreadStore(0) // default cap

	s.Append("alice", AgentChiefOfStaff, Message{Role: RoleUser, Content: "hello"})
	s.Append("alice", AgentChiefOfStaff, Message{Role: RoleAssistant, Content: "hi"})

	got := s.Snapshot("alice", AgentChiefOfStaff)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Content != "hello" || got[1].Content != "hi" {
		t.Errorf("messages out of order: %+v", got)
	}
}

func TestThreadStore_IsolatesPerOperator(t *testing.T) {
	s := NewThreadStore(0)
	s.Append("alice", AgentChiefOfStaff, Message{Role: RoleUser, Content: "alice-msg"})
	s.Append("bob", AgentChiefOfStaff, Message{Role: RoleUser, Content: "bob-msg"})

	if got := s.Snapshot("alice", AgentChiefOfStaff); len(got) != 1 || got[0].Content != "alice-msg" {
		t.Errorf("alice's thread cross-contaminated: %+v", got)
	}
	if got := s.Snapshot("bob", AgentChiefOfStaff); len(got) != 1 || got[0].Content != "bob-msg" {
		t.Errorf("bob's thread cross-contaminated: %+v", got)
	}
}

func TestThreadStore_IsolatesPerAgent(t *testing.T) {
	s := NewThreadStore(0)
	s.Append("alice", AgentChiefOfStaff, Message{Role: RoleUser, Content: "cos"})
	s.Append("alice", AgentMarketing, Message{Role: RoleUser, Content: "mk"})

	if got := s.Snapshot("alice", AgentChiefOfStaff); len(got) != 1 || got[0].Content != "cos" {
		t.Errorf("CoS thread polluted: %+v", got)
	}
	if got := s.Snapshot("alice", AgentMarketing); len(got) != 1 || got[0].Content != "mk" {
		t.Errorf("Marketing thread polluted: %+v", got)
	}
}

func TestThreadStore_TruncatesAtCap(t *testing.T) {
	s := NewThreadStore(3)
	for i := 0; i < 5; i++ {
		s.Append("alice", AgentChiefOfStaff, Message{Role: RoleUser, Content: strconv.Itoa(i)})
	}
	got := s.Snapshot("alice", AgentChiefOfStaff)
	if len(got) != 3 {
		t.Fatalf("expected cap=3, got %d", len(got))
	}
	// Oldest two were dropped; messages 2, 3, 4 remain.
	for i, msg := range got {
		want := strconv.Itoa(i + 2)
		if msg.Content != want {
			t.Errorf("at index %d: want %q, got %q", i, want, msg.Content)
		}
	}
}

func TestThreadStore_SnapshotIsDefensiveCopy(t *testing.T) {
	s := NewThreadStore(0)
	s.Append("alice", AgentChiefOfStaff, Message{Role: RoleUser, Content: "first"})
	snap := s.Snapshot("alice", AgentChiefOfStaff)
	snap[0].Content = "MUTATED"

	again := s.Snapshot("alice", AgentChiefOfStaff)
	if again[0].Content != "first" {
		t.Errorf("snapshot mutation leaked back into store: %+v", again)
	}
}

func TestThreadStore_SnapshotOfUnknownThreadReturnsEmpty(t *testing.T) {
	s := NewThreadStore(0)
	got := s.Snapshot("never-seen", AgentChiefOfStaff)
	if got == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestThreadStore_Reset(t *testing.T) {
	s := NewThreadStore(0)
	s.Append("alice", AgentChiefOfStaff, Message{Role: RoleUser, Content: "x"})
	s.Append("alice", AgentMarketing, Message{Role: RoleUser, Content: "y"})

	s.Reset("alice", AgentChiefOfStaff)
	if len(s.Snapshot("alice", AgentChiefOfStaff)) != 0 {
		t.Errorf("expected CoS thread cleared")
	}
	if len(s.Snapshot("alice", AgentMarketing)) != 1 {
		t.Errorf("expected Marketing thread intact")
	}

	// Empty args = full wipe.
	s.Reset("", "")
	if len(s.Snapshot("alice", AgentMarketing)) != 0 {
		t.Errorf("full reset failed to clear Marketing thread")
	}
}
