// Package secrets provides durable storage for credentials the daemon never
// wants to write to disk in plaintext: device tokens for paired stores
// (internal/store row.token_ref), model-provider API keys (model_providers
// row.secret_ref), and any future secret-shaped material.
//
// The implementation wraps github.com/zalando/go-keyring, which delegates to
// the OS keychain on macOS (via the `security` CLI), Credential Manager on
// Windows (wincred syscalls), and the Secret Service on Linux (DBus). All
// pure-Go — preserves the no-CGO single-binary promise.
//
// On a fresh daemon start, callers invoke Open() which probes the backend
// with a sentinel write/read/delete. A failed probe is fatal: v0.1 refuses
// to run without a working secret store. A file-backed fallback for
// headless Linux without gnome-keyring/kwallet is on the v0.2 list.
package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// serviceName scopes WooAgent OS entries inside the host keychain. macOS
// surfaces it as the keychain "service" attribute; Linux uses it as the
// Secret Service collection's schema name.
const serviceName = "WooAgent OS"

// ErrNotFound is returned by Get and Delete when the requested key is not
// present in the keychain.
var ErrNotFound = errors.New("secrets: key not found")

// Store is the abstract surface the daemon's stores and model_providers
// handlers depend on. Tests inject a fake; production wiring uses Open.
type Store interface {
	Set(ctx context.Context, key, value string) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

// osKeyring is the concrete Store backed by zalando/go-keyring. The context
// argument is accepted on every method for parity with future async backends
// but ignored today — keychain access is synchronous and fast.
type osKeyring struct{}

func (osKeyring) Set(_ context.Context, key, value string) error {
	return keyring.Set(serviceName, key, value)
}

func (osKeyring) Get(_ context.Context, key string) (string, error) {
	v, err := keyring.Get(serviceName, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (osKeyring) Delete(_ context.Context, key string) error {
	err := keyring.Delete(serviceName, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// Open returns the OS-backed Store after a write/read/delete probe of a
// sentinel key. A failed probe means the host keychain is unavailable — the
// daemon should refuse to start rather than persist secrets nowhere.
func Open() (Store, error) {
	s := osKeyring{}
	const probe = "_wooagent_probe"
	ctx := context.Background()
	if err := s.Set(ctx, probe, "ok"); err != nil {
		return nil, fmt.Errorf("secrets: keychain unavailable (write probe): %w", err)
	}
	if _, err := s.Get(ctx, probe); err != nil {
		return nil, fmt.Errorf("secrets: keychain unavailable (read probe): %w", err)
	}
	if err := s.Delete(ctx, probe); err != nil {
		// Failure to clean up the probe is non-fatal: the keychain is
		// working, we just leaked one row. Log via the caller's facility.
		return s, nil
	}
	return s, nil
}
