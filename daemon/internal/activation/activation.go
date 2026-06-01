// Package activation sends the opt-in, anonymized "first approve" activation
// ping — the daemon's only outbound-analytics surface. Off by default; fires
// once per install when explicitly enabled. See the 2026-06-01 activation-ping
// design.
package activation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config gates the ping. Both fields must be set for anything to fire.
type Config struct {
	Enabled bool
	URL     string
}

// ConfigFromEnv reads WOOAGENT_TELEMETRY_ENABLED (default false) and
// WOOAGENT_TELEMETRY_URL (default empty). An empty URL means no ping even when
// enabled — defense in depth against pinging a placeholder destination.
func ConfigFromEnv() Config {
	enabled := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WOOAGENT_TELEMETRY_ENABLED"))) {
	case "1", "true", "yes", "on":
		enabled = true
	}
	return Config{
		Enabled: enabled,
		URL:     strings.TrimSpace(os.Getenv("WOOAGENT_TELEMETRY_URL")),
	}
}

const installIDKey = "install_id"

// metaGet returns the daemon_meta value for key, or "" if absent.
func metaGet(ctx context.Context, db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM daemon_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("meta get %s: %w", key, err)
	}
	return v, nil
}

// metaSet upserts a daemon_meta key.
func metaSet(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT OR REPLACE INTO daemon_meta (key, value, updated_at) VALUES (?,?,?)`,
		key, value, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("meta set %s: %w", key, err)
	}
	return nil
}

// installID returns the anonymous install UUID, generating + persisting it on
// first call. Stable across restarts; not derived from anything identifying.
func installID(ctx context.Context, db *sql.DB) (string, error) {
	id, err := metaGet(ctx, db, installIDKey)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	id = uuid.NewString()
	if err := metaSet(ctx, db, installIDKey, id); err != nil {
		return "", err
	}
	return id, nil
}

const pingedKey = "activation_pinged_at"

// pinger is the HTTP seam so tests inject a fake (no real network).
type pinger interface {
	send(ctx context.Context, url string, body []byte) (int, error)
}

type httpPinger struct{}

func (httpPinger) send(ctx context.Context, url string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

type pingPayload struct {
	Event         string `json:"event"`
	InstallID     string `json:"install_id"`
	DaemonVersion string `json:"daemon_version"`
	TS            string `json:"ts"`
}

// MaybePingFirstApprove sends the first-approve activation ping using the real
// HTTP sender. Callers MUST invoke it in a goroutine with a detached context
// (e.g. `go activation.MaybePingFirstApprove(context.Background(), ...)`); it
// applies its own timeout. Errors are logged, never returned.
func MaybePingFirstApprove(ctx context.Context, db *sql.DB, cfg Config, version string) {
	maybePing(ctx, db, cfg, version, httpPinger{})
}

// maybePing is the seam-testable core. Synchronous: guard → install id →
// POST → stamp on 2xx. activation_pinged_at is stamped only on success, so a
// failed ping retries on the next approve and the event lands exactly once.
func maybePing(ctx context.Context, db *sql.DB, cfg Config, version string, p pinger) {
	if !cfg.Enabled || cfg.URL == "" {
		return
	}
	pinged, err := metaGet(ctx, db, pingedKey)
	if err != nil {
		slog.Default().Warn("activation: read pinged flag failed", "err", err)
		return
	}
	if pinged != "" {
		return
	}
	id, err := installID(ctx, db)
	if err != nil {
		slog.Default().Warn("activation: install id failed", "err", err)
		return
	}
	body, err := json.Marshal(pingPayload{
		Event:         "first_approve",
		InstallID:     id,
		DaemonVersion: version,
		TS:            time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.Default().Warn("activation: marshal payload failed", "err", err)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := p.send(cctx, cfg.URL, body)
	if err != nil {
		slog.Default().Warn("activation: ping failed", "err", err)
		return
	}
	if status < 200 || status >= 300 {
		slog.Default().Warn("activation: ping non-2xx", "status", status)
		return
	}
	if err := metaSet(ctx, db, pingedKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
		slog.Default().Warn("activation: stamp pinged failed", "err", err)
		return
	}
	slog.Default().Info("activation: first-approve ping sent")
}
