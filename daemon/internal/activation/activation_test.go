package activation

import (
	"context"
	"database/sql"
	"encoding/json"
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

type fakePinger struct {
	calls  int
	bodies [][]byte
	status int
	err    error
}

func (f *fakePinger) send(ctx context.Context, url string, body []byte) (int, error) {
	f.calls++
	f.bodies = append(f.bodies, body)
	if f.err != nil {
		return 0, f.err
	}
	return f.status, nil
}

func TestMaybePing_DisabledOrNoURL(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	fp := &fakePinger{status: 200}
	maybePing(ctx, db, Config{Enabled: false, URL: "https://x"}, "0.1.0", fp)
	maybePing(ctx, db, Config{Enabled: true, URL: ""}, "0.1.0", fp)
	if fp.calls != 0 {
		t.Errorf("disabled/no-url should not send; calls=%d", fp.calls)
	}
}

func TestMaybePing_FiresOnceAndStamps(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	fp := &fakePinger{status: 200}
	cfg := Config{Enabled: true, URL: "https://x.example/ping"}
	maybePing(ctx, db, cfg, "1.2.3", fp)
	if fp.calls != 1 {
		t.Fatalf("calls = %d, want 1", fp.calls)
	}
	var got map[string]any
	if err := json.Unmarshal(fp.bodies[0], &got); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if got["event"] != "first_approve" || got["daemon_version"] != "1.2.3" {
		t.Errorf("payload = %v", got)
	}
	if got["install_id"] == "" || got["install_id"] == nil {
		t.Errorf("install_id missing")
	}
	if _, ok := got["ts"]; !ok {
		t.Errorf("ts missing")
	}
	if len(got) != 4 {
		t.Errorf("payload has %d keys, want exactly 4 (no PII leak): %v", len(got), got)
	}
	maybePing(ctx, db, cfg, "1.2.3", fp)
	if fp.calls != 1 {
		t.Errorf("already-pinged should skip; calls=%d", fp.calls)
	}
	if at, _ := metaGet(ctx, db, "activation_pinged_at"); at == "" {
		t.Errorf("activation_pinged_at not stamped")
	}
}

func TestMaybePing_FailureLeavesUnstamped(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	cfg := Config{Enabled: true, URL: "https://x.example/ping"}
	maybePing(ctx, db, cfg, "1.0.0", &fakePinger{status: 500})
	if at, _ := metaGet(ctx, db, "activation_pinged_at"); at != "" {
		t.Errorf("non-2xx should not stamp")
	}
	maybePing(ctx, db, cfg, "1.0.0", &fakePinger{err: errSend})
	if at, _ := metaGet(ctx, db, "activation_pinged_at"); at != "" {
		t.Errorf("transport error should not stamp")
	}
	fp := &fakePinger{status: 200}
	maybePing(ctx, db, cfg, "1.0.0", fp)
	if at, _ := metaGet(ctx, db, "activation_pinged_at"); at == "" {
		t.Errorf("success should stamp")
	}
	maybePing(ctx, db, cfg, "1.0.0", fp)
	if fp.calls != 1 {
		t.Errorf("after success, no further sends; calls=%d", fp.calls)
	}
}

var errSend = errSendType("send failed")

type errSendType string

func (e errSendType) Error() string { return string(e) }
