// Package activation sends the opt-in, anonymized "first approve" activation
// ping — the daemon's only outbound-analytics surface. Off by default; fires
// once per install when explicitly enabled. See the 2026-06-01 activation-ping
// design.
package activation

import (
	"context"
	"database/sql"
	"fmt"
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
