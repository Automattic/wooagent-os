package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestRoundtrip walks the Set/Get/Delete cycle against the in-memory backend
// keyring.MockInit installs. Verifies that ErrNotFound surfaces from Get
// after Delete (the proposal calls this out explicitly).
func TestRoundtrip(t *testing.T) {
	keyring.MockInit()
	s := osKeyring{}
	ctx := context.Background()

	if err := s.Set(ctx, "wooagent.test.key", "secret-value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	v, err := s.Get(ctx, "wooagent.test.key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "secret-value" {
		t.Errorf("Get returned %q, want %q", v, "secret-value")
	}

	if err := s.Delete(ctx, "wooagent.test.key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := s.Get(ctx, "wooagent.test.key"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete returned %v, want ErrNotFound", err)
	}
}

// TestGetMissing returns ErrNotFound for a key that was never written.
func TestGetMissing(t *testing.T) {
	keyring.MockInit()
	s := osKeyring{}
	if _, err := s.Get(context.Background(), "wooagent.test.never-set"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on missing key returned %v, want ErrNotFound", err)
	}
}

// TestDeleteMissing returns ErrNotFound rather than nil so callers can
// distinguish "deleted" from "wasn't there." The handlers in /v1/stores
// and /v1/model-providers use this to decide whether the row + secret
// were genuinely linked.
func TestDeleteMissing(t *testing.T) {
	keyring.MockInit()
	s := osKeyring{}
	if err := s.Delete(context.Background(), "wooagent.test.never-set"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete on missing key returned %v, want ErrNotFound", err)
	}
}

// TestOverwrite confirms a second Set on the same key replaces the value.
// The proposal's PATCH /v1/model-providers/:id rotation path depends on this.
func TestOverwrite(t *testing.T) {
	keyring.MockInit()
	s := osKeyring{}
	ctx := context.Background()

	if err := s.Set(ctx, "wooagent.test.key", "original"); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	if err := s.Set(ctx, "wooagent.test.key", "rotated"); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	v, err := s.Get(ctx, "wooagent.test.key")
	if err != nil {
		t.Fatalf("Get after rotate: %v", err)
	}
	if v != "rotated" {
		t.Errorf("Get returned %q, want %q", v, "rotated")
	}
}
