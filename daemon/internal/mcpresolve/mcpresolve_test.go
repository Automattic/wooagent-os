package mcpresolve

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// fakeSecrets is an in-memory secrets.Store.
type fakeSecrets struct {
	vals map[string]string
	err  error
}

func (f *fakeSecrets) Set(_ context.Context, k, v string) error { f.vals[k] = v; return nil }
func (f *fakeSecrets) Delete(_ context.Context, k string) error { delete(f.vals, k); return nil }
func (f *fakeSecrets) Get(_ context.Context, k string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.vals[k]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st.DB
}

func seedPairedStore(t *testing.T, db *sql.DB, id, host, tokenRef string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO stores (id, url, mcp_endpoint, status, token_ref, paired_at, created_at, updated_at)
		VALUES (?, ?, ?, 'paired', ?, '2026-08-02T15:40:00Z', '2026-08-02T15:39:00Z', '2026-08-02T15:40:00Z')`,
		id, "https://"+host, "https://"+host+"/wp-json/mcp/mcp-adapter-default-server", tokenRef)
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
}

func setEnvStore(t *testing.T, host string) {
	t.Helper()
	t.Setenv("WOOAGENT_MCP_URL", "https://"+host+"/wp-json/mcp/mcp-adapter-default-server")
	t.Setenv("WOOAGENT_MCP_USER", "op@example.com")
	t.Setenv("WOOAGENT_MCP_APP_PASSWORD", "abcd efgh ijkl")
}

// The exact failure from DSGWOO-1470: an operator re-pairs to a new store
// while WOOAGENT_MCP_URL still names the old, decommissioned one. The
// paired store must win, or every run keeps hitting the dead host while the
// UI reports the new one as healthy.
func TestResolveMCPTarget_PairedStoreBeatsStaleEnv(t *testing.T) {
	db := openTestDB(t)
	seedPairedStore(t, db, "store_new", "new-store.example.com", "ref-new")
	setEnvStore(t, "old-store.example.com")
	sec := &fakeSecrets{vals: map[string]string{"ref-new": "device-token"}}

	var buf bytes.Buffer
	got, ok := Resolve(context.Background(), db, sec, &buf)
	if !ok {
		t.Fatal("expected a resolved target")
	}
	if got.Source != SourcePairedStore {
		t.Errorf("Source = %q, want %q", got.Source, SourcePairedStore)
	}
	if !strings.Contains(got.Config.Endpoint, "new-store.example.com") {
		t.Errorf("Endpoint = %q, want the paired store", got.Config.Endpoint)
	}
	if got.Config.BearerToken != "device-token" {
		t.Errorf("BearerToken = %q, want the keychain token", got.Config.BearerToken)
	}
	// Divergence must be reported — silence here is the whole defect.
	out := buf.String()
	if !strings.Contains(out, "does not match the paired store") {
		t.Errorf("expected a divergence warning, got: %q", out)
	}
	if !strings.Contains(out, "old-store.example.com") || !strings.Contains(out, "new-store.example.com") {
		t.Errorf("warning should name both hosts, got: %q", out)
	}
}

// Same host in both places is the normal configured state, not a problem.
func TestResolveMCPTarget_NoWarningWhenHostsAgree(t *testing.T) {
	db := openTestDB(t)
	seedPairedStore(t, db, "store_a", "shop.example.com", "ref-a")
	setEnvStore(t, "shop.example.com")
	sec := &fakeSecrets{vals: map[string]string{"ref-a": "tok"}}

	var buf bytes.Buffer
	got, ok := Resolve(context.Background(), db, sec, &buf)
	if !ok || got.Source != SourcePairedStore {
		t.Fatalf("got %+v ok=%v, want paired store", got, ok)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no warning when hosts agree, got: %q", buf.String())
	}
}

// Headless / CI: nothing paired, env drives.
func TestResolveMCPTarget_EnvFallback(t *testing.T) {
	db := openTestDB(t)
	setEnvStore(t, "headless.example.com")

	got, ok := Resolve(context.Background(), db, &fakeSecrets{vals: map[string]string{}}, io.Discard)
	if !ok {
		t.Fatal("expected env fallback to resolve")
	}
	if got.Source != SourceEnv {
		t.Errorf("Source = %q, want %q", got.Source, SourceEnv)
	}
	if got.Config.Username != "op@example.com" {
		t.Errorf("Username = %q", got.Config.Username)
	}
	// App passwords are shown spaced in wp-admin; spaces must be stripped.
	if got.Config.Password != "abcdefghijkl" {
		t.Errorf("Password = %q, want spaces stripped", got.Config.Password)
	}
}

func TestResolveMCPTarget_NeitherConfigured(t *testing.T) {
	t.Setenv("WOOAGENT_MCP_URL", "")
	t.Setenv("WOOAGENT_MCP_USER", "")
	t.Setenv("WOOAGENT_MCP_APP_PASSWORD", "")

	if _, ok := Resolve(context.Background(), openTestDB(t), &fakeSecrets{vals: map[string]string{}}, io.Discard); ok {
		t.Error("expected ok=false when nothing is configured")
	}
}

// A paired row whose keychain entry is gone can't authenticate. Falling back
// to env beats handing back a client that will 401 on every call.
func TestResolveMCPTarget_UnreadableTokenFallsBackToEnv(t *testing.T) {
	db := openTestDB(t)
	seedPairedStore(t, db, "store_x", "paired.example.com", "ref-missing")
	setEnvStore(t, "env.example.com")
	sec := &fakeSecrets{vals: map[string]string{}} // ref-missing absent

	var buf bytes.Buffer
	got, ok := Resolve(context.Background(), db, sec, &buf)
	if !ok {
		t.Fatal("expected env fallback")
	}
	if got.Source != SourceEnv {
		t.Errorf("Source = %q, want %q", got.Source, SourceEnv)
	}
	if !strings.Contains(buf.String(), "token unreadable") {
		t.Errorf("expected an explanatory warning, got: %q", buf.String())
	}
}

// An unpaired row must not be picked up.
func TestResolveMCPTarget_IgnoresUnpairedStore(t *testing.T) {
	db := openTestDB(t)
	seedPairedStore(t, db, "store_u", "unpaired.example.com", "ref-u")
	if _, err := db.Exec(`UPDATE stores SET status='unpaired' WHERE id='store_u'`); err != nil {
		t.Fatalf("unpair: %v", err)
	}
	t.Setenv("WOOAGENT_MCP_URL", "")
	t.Setenv("WOOAGENT_MCP_USER", "")
	t.Setenv("WOOAGENT_MCP_APP_PASSWORD", "")

	if _, ok := Resolve(context.Background(), db, &fakeSecrets{vals: map[string]string{"ref-u": "t"}}, io.Discard); ok {
		t.Error("an unpaired store should not resolve")
	}
}

// The live re-pair path: the daemon is already running against one store,
// the operator pairs a different one, and the shared client follows without
// a restart.
func TestReconcileMCPOnce_FollowsRepair(t *testing.T) {
	db := openTestDB(t)
	seedPairedStore(t, db, "store_1", "first.example.com", "ref-1")
	t.Setenv("WOOAGENT_MCP_URL", "")
	t.Setenv("WOOAGENT_MCP_USER", "")
	t.Setenv("WOOAGENT_MCP_APP_PASSWORD", "")
	sec := &fakeSecrets{vals: map[string]string{"ref-1": "tok-1", "ref-2": "tok-2"}}

	target, ok := Resolve(context.Background(), db, sec, io.Discard)
	if !ok {
		t.Fatal("initial resolve failed")
	}
	c := mcp.NewClient(target.Config)

	// Nothing changed yet — reconciling must be a quiet no-op.
	var buf bytes.Buffer
	ReconcileOnce(context.Background(), c, db, sec, &buf)
	if buf.Len() != 0 {
		t.Errorf("no-op reconcile should be silent, got: %q", buf.String())
	}

	// Operator pairs a second store; it becomes the most recent.
	if _, err := db.Exec(`UPDATE stores SET status='unpaired' WHERE id='store_1'`); err != nil {
		t.Fatalf("unpair first: %v", err)
	}
	seedPairedStore(t, db, "store_2", "second.example.com", "ref-2")

	ReconcileOnce(context.Background(), c, db, sec, &buf)

	if !strings.Contains(c.Endpoint(), "second.example.com") {
		t.Errorf("client endpoint = %q, want the newly paired store", c.Endpoint())
	}
	if !strings.Contains(buf.String(), "connected store changed") {
		t.Errorf("expected a store-changed log line, got: %q", buf.String())
	}
}

// If everything is unpaired and env is empty, keep the existing connection
// rather than blanking it — personas give a clearer skip than a client
// pointed at nothing.
func TestReconcileMCPOnce_KeepsClientWhenNothingResolves(t *testing.T) {
	db := openTestDB(t)
	t.Setenv("WOOAGENT_MCP_URL", "")
	t.Setenv("WOOAGENT_MCP_USER", "")
	t.Setenv("WOOAGENT_MCP_APP_PASSWORD", "")

	c := mcp.NewClient(mcp.Config{Endpoint: "https://still.example.com/mcp", BearerToken: "t"})
	ReconcileOnce(context.Background(), c, db, &fakeSecrets{vals: map[string]string{}}, io.Discard)

	if c.Endpoint() != "https://still.example.com/mcp" {
		t.Errorf("endpoint = %q, want the previous target preserved", c.Endpoint())
	}
}

func TestHostOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.example.com/wp-json/mcp/x": "a.example.com",
		"https://b.example.com":               "b.example.com",
		"":                                    "",
		"not a url":                           "not a url",
	} {
		if got := HostOf(in); got != want {
			t.Errorf("HostOf(%q) = %q, want %q", in, got, want)
		}
	}
}
