package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is a thin wrapper around *sql.DB that carries the applied migration
// version. Callers reach for .DB for query/exec; the wrapper exists so future
// cross-cutting concerns (tracing, metrics) have a single place to hook in.
type Store struct {
	DB *sql.DB
}

// Open opens (or creates) a SQLite database at dsn, enables foreign keys and
// WAL mode, applies any pending migrations embedded in the binary, and returns
// a ready-to-use Store.
func Open(ctx context.Context, dsn string) (*Store, error) {
	// modernc.org/sqlite applies most pragmas per *connection*, and
	// database/sql hands out a pool of them — so a pragma issued once via Exec
	// only sticks on whichever connection happened to serve it. foreign_keys
	// in particular MUST be set on every connection or ON DELETE CASCADE
	// silently no-ops, orphaning child rows (e.g. abilities) when a parent
	// store is deleted. Pass connection-scoped pragmas through
	// the DSN so the driver re-applies them on every Open. journal_mode and
	// synchronous stay below: journal_mode is a persistent, file-level setting
	// (one Exec suffices) and is invalid for :memory:, so it must not go in the
	// DSN pragma list.
	db, err := sql.Open("sqlite", withConnPragmas(dsn))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dsn, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}

	// Tighten the DB file to 0600 — it carries customer/order/proposal/audit
	// data that no other local user should be able to read. ErrNotExist is the
	// `:memory:` test path; ignore it. The parent directory is locked to 0700
	// by config.EnsureDirs, so WAL/SHM sidecar files (created lazily by the
	// driver) are protected by directory-level access control.
	if err := os.Chmod(dsn, 0o600); err != nil && !errors.Is(err, fs.ErrNotExist) {
		_ = db.Close()
		return nil, fmt.Errorf("chmod sqlite file %s: %w", dsn, err)
	}

	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

// withConnPragmas appends the connection-scoped pragmas to dsn as modernc
// _pragma query parameters, so the driver applies them on every pooled
// connection (not just the one a post-open Exec happens to use). Works for
// bare paths, file: URIs, and :memory:.
func withConnPragmas(dsn string) string {
	const pragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + pragmas
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version    TEXT PRIMARY KEY,
        applied_at TEXT NOT NULL
    )`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := s.DB.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = rows.Close()

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}
	return nil
}
