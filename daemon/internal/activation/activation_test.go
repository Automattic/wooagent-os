package activation

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(context.Background(),
		`CREATE TABLE daemon_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("WOOAGENT_TELEMETRY_ENABLED", "1")
	t.Setenv("WOOAGENT_TELEMETRY_URL", " https://x.example/ping ")
	c := ConfigFromEnv()
	if !c.Enabled || c.URL != "https://x.example/ping" {
		t.Errorf("got %+v", c)
	}
	t.Setenv("WOOAGENT_TELEMETRY_ENABLED", "")
	if ConfigFromEnv().Enabled {
		t.Errorf("empty enabled should be false")
	}
}

func TestInstallID_StableAndGenerated(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	id1, err := installID(ctx, db)
	if err != nil {
		t.Fatalf("installID: %v", err)
	}
	if id1 == "" {
		t.Fatal("install id should be generated")
	}
	id2, _ := installID(ctx, db)
	if id1 != id2 {
		t.Errorf("install id not stable: %q != %q", id1, id2)
	}
}

func TestMetaGetSet(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	got, err := metaGet(ctx, db, "missing")
	if err != nil || got != "" {
		t.Errorf("missing key → (%q,%v), want (\"\",nil)", got, err)
	}
	if err := metaSet(ctx, db, "k", "v1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := metaSet(ctx, db, "k", "v2"); err != nil {
		t.Fatalf("set2: %v", err)
	}
	if got, _ := metaGet(ctx, db, "k"); got != "v2" {
		t.Errorf("get = %q, want v2 (upsert)", got)
	}
}
