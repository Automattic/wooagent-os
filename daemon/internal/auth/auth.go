package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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
// embedded UI's auto-auth path. The daemon mints one of these on every
// startup so the React UI loaded from the daemon's own host can call
// /v1/* without the operator pasting a bearer token.
const UISessionTokenName = "ui-session"

// MintUISession deletes any prior ui-session row and mints a fresh one,
// returning the plaintext to the caller. The plaintext is held in
// process memory for the daemon run and templated into index.html via
// the uiassets handler so the embedded UI auto-connects.
//
// We rotate per-run rather than persisting plaintext (the auth_tokens
// table only stores hashes) because the UI session token is ephemeral
// to "this daemon process" — operator-issued long-lived tokens (the
// kind minted by `wooagent init` / `wooagent auth token create`)
// survive restarts; this one doesn't need to.
func (m *Manager) MintUISession(ctx context.Context) (string, error) {
	if _, err := m.DB.ExecContext(ctx,
		`DELETE FROM auth_tokens WHERE name = ?`, UISessionTokenName,
	); err != nil {
		return "", fmt.Errorf("clear prior ui-session: %w", err)
	}
	return m.Mint(ctx, UISessionTokenName)
}

func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
