package auth

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const authTokensDDL = `
CREATE TABLE auth_tokens (
    token_hash      TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    last_used_at    TEXT
);`

// newTestManager returns a Manager backed by an in-memory SQLite database
// with the auth_tokens table already created.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(authTokensDDL); err != nil {
		t.Fatalf("apply auth_tokens ddl: %v", err)
	}
	return New(db)
}

func TestValidate_HappyPath(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	tok, err := m.Mint(ctx, "test-token")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	name, err := m.Validate(ctx, tok)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if name != "test-token" {
		t.Errorf("Validate returned name %q, want %q", name, "test-token")
	}
}

func TestValidate_WrongToken(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	_, err := m.Mint(ctx, "my-token")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	_, err = m.Validate(ctx, "wo_pat_doesnotexist")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Validate with bad token: got %v, want ErrInvalidToken", err)
	}
}

func TestValidate_EmptyToken(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	_, err := m.Validate(ctx, "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Validate with empty token: got %v, want ErrInvalidToken", err)
	}
}

func TestValidate_UISessionName(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	tokenFile := filepath.Join(t.TempDir(), "ui-session.token")

	tok, err := m.EnsureUISession(ctx, tokenFile)
	if err != nil {
		t.Fatalf("EnsureUISession: %v", err)
	}

	name, err := m.Validate(ctx, tok)
	if err != nil {
		t.Fatalf("Validate ui-session: %v", err)
	}
	if name != UISessionTokenName {
		t.Errorf("Validate returned name %q, want %q", name, UISessionTokenName)
	}
}

func TestEnsureUISession_PersistsAcrossCalls(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	tokenFile := filepath.Join(t.TempDir(), "ui-session.token")

	first, err := m.EnsureUISession(ctx, tokenFile)
	if err != nil {
		t.Fatalf("EnsureUISession (mint): %v", err)
	}
	second, err := m.EnsureUISession(ctx, tokenFile)
	if err != nil {
		t.Fatalf("EnsureUISession (reuse): %v", err)
	}
	if first != second {
		t.Errorf("second call should reuse the persisted token, got fresh one")
	}
}

func TestEnsureUISession_RotatesOnOrphanFile(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	tokenFile := filepath.Join(t.TempDir(), "ui-session.token")

	// Pre-write a plaintext that doesn't correspond to any DB row.
	if err := os.WriteFile(tokenFile, []byte("wo_pat_orphan\n"), 0o600); err != nil {
		t.Fatalf("write orphan file: %v", err)
	}
	tok, err := m.EnsureUISession(ctx, tokenFile)
	if err != nil {
		t.Fatalf("EnsureUISession with orphan: %v", err)
	}
	if tok == "wo_pat_orphan" {
		t.Errorf("should have rotated past orphan token, got the orphan back")
	}
	if name, err := m.Validate(ctx, tok); err != nil || name != UISessionTokenName {
		t.Errorf("rotated token should validate as %q, got name=%q err=%v",
			UISessionTokenName, name, err)
	}
}

func TestValidate_MultipleTokens_CorrectNameReturned(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	_, err := m.Mint(ctx, "first-token")
	if err != nil {
		t.Fatalf("Mint first: %v", err)
	}
	second, err := m.Mint(ctx, "second-token")
	if err != nil {
		t.Fatalf("Mint second: %v", err)
	}

	name, err := m.Validate(ctx, second)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if name != "second-token" {
		t.Errorf("Validate returned name %q, want %q", name, "second-token")
	}
}
