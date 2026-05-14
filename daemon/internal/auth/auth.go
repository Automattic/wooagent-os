package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// TokenPrefix is the human-readable prefix used on every minted token. It
// gives operators a quick "yes that's a WooAgent token" signal when they see
// one in logs, env vars, or UI paste fields.
const TokenPrefix = "wo_pat_"

var ErrInvalidToken = errors.New("invalid auth token")

// Manager issues and validates bearer tokens against the auth_tokens table.
// The plaintext token is never persisted — only a SHA-256 hash is. Operators
// see the plaintext once (at mint time) and are expected to paste it into the
// UI immediately.
type Manager struct {
	DB *sql.DB
}

func New(db *sql.DB) *Manager {
	return &Manager{DB: db}
}

// Mint creates a new token with the given friendly name and returns the
// plaintext form to the caller. Keep the plaintext brief — it's copy-pasted.
func (m *Manager) Mint(ctx context.Context, name string) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	plaintext := TokenPrefix + hex.EncodeToString(raw)
	hash := hashToken(plaintext)
	if _, err := m.DB.ExecContext(ctx,
		`INSERT INTO auth_tokens(token_hash, name, created_at) VALUES(?, ?, ?)`,
		hash, name, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	return plaintext, nil
}

// Validate returns the friendly name of the matching token if the
// supplied plaintext is recognized; otherwise ErrInvalidToken. The
// name is the operator identity stashed in audit rows for any
// mutation the bearer performs. On success it updates last_used_at.
func (m *Manager) Validate(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", ErrInvalidToken
	}
	hash := hashToken(plaintext)
	var name string
	err := m.DB.QueryRowContext(ctx,
		`SELECT name FROM auth_tokens WHERE token_hash = ?`, hash,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("lookup token: %w", err)
	}
	_, _ = m.DB.ExecContext(ctx, `UPDATE auth_tokens SET last_used_at = ? WHERE token_hash = ?`,
		time.Now().UTC().Format(time.RFC3339), hash)
	return name, nil
}

// AnyTokenExists reports whether at least one auth token is present. Used by
// `wooagent init` to skip minting when the DB already has one.
func (m *Manager) AnyTokenExists(ctx context.Context) (bool, error) {
	var n int
	if err := m.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_tokens`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// UISessionTokenName is the well-known auth_tokens.name reserved for the
// embedded UI's auto-auth path. The daemon mints one of these on first
// use (init or first `wooagent run`) and persists the plaintext to
// `tokenFilePath` (mode 0600) so subsequent restarts reuse the same
// token. Without persistence, every restart silently invalidated any
// open browser session — internal testers had no way to recover.
const UISessionTokenName = "ui-session"

// EnsureUISession returns the embedded UI's bearer token, minting and
// persisting a fresh one only when needed. Order of operations:
//
//  1. Read tokenFilePath. If present and the plaintext hashes to an
//     auth_tokens row named `ui-session`, return the plaintext — no
//     DB change. This is the steady-state path: same token across
//     restarts, same embedded `window.__WOOAGENT_TOKEN__`, same auth
//     for browsers that loaded the UI before the restart.
//  2. Otherwise (file missing, unreadable, empty, or hash doesn't
//     match a row): delete any prior `ui-session` rows, mint a fresh
//     plaintext, write it to tokenFilePath with mode 0600, return it.
//
// The plaintext on disk lives alongside `wooagent.db` and is protected
// by the same OS-level home-dir permissions; storing it plaintext is
// roughly equivalent in security to the daemon templating it into
// index.html every restart. Operator-issued long-lived tokens (from
// `wooagent init` / `wooagent auth token create`) are unaffected.
//
// To force rotation: delete the file (and optionally the DB row) before
// starting the daemon, or use a future `wooagent auth rotate` command.
func (m *Manager) EnsureUISession(ctx context.Context, tokenFilePath string) (string, error) {
	if plaintext, ok := readUISessionFile(tokenFilePath); ok {
		hash := hashToken(plaintext)
		var name string
		err := m.DB.QueryRowContext(ctx,
			`SELECT name FROM auth_tokens WHERE token_hash = ?`, hash,
		).Scan(&name)
		if err == nil && name == UISessionTokenName {
			return plaintext, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("verify ui-session: %w", err)
		}
		// File is orphan (no matching row) or matches a row with a
		// different name. Treat as missing and rotate.
	}

	if _, err := m.DB.ExecContext(ctx,
		`DELETE FROM auth_tokens WHERE name = ?`, UISessionTokenName,
	); err != nil {
		return "", fmt.Errorf("clear prior ui-session: %w", err)
	}
	plaintext, err := m.Mint(ctx, UISessionTokenName)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(tokenFilePath, []byte(plaintext+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist ui-session: %w", err)
	}
	return plaintext, nil
}

// readUISessionFile returns the stored plaintext and ok=true when the
// file is present, readable, and non-empty after trimming.
func readUISessionFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	plaintext := strings.TrimSpace(string(data))
	if plaintext == "" {
		return "", false
	}
	return plaintext, true
}

func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
