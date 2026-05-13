package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// newRunsTestRig extends the base newTestRig with the /v1/runs routes
// registered on the test server. Returns the server plus a dedicated
// httptest.Server that includes those routes.
func newRunsTestRig(t *testing.T) (*Server, *httptest.Server, *store.Store) {
	t.Helper()
	srv, _, st := newTestRig(t, nil)

	r := chi.NewRouter()
	r.Get("/v1/runs", srv.handleListRuns)
	r.Get("/v1/runs/{id}", srv.handleGetRun)
	r.Post("/v1/runs", srv.handleCreateRun)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return srv, ts, st
}

func TestListRuns_FiltersByPersona(t *testing.T) {
	_, ts, st := newRunsTestRig(t)
	defer ts.Close()

	fixed := time.Now().UTC().Format(time.RFC3339)
	// Seed two runs.
	if _, err := st.DB.Exec(
		`INSERT INTO runs(id, persona, trigger, status, scheduled_at, created_at) VALUES('r1','marketing','tick','succeeded',?,?)`,
		fixed, fixed,
	); err != nil {
		t.Fatalf("seed r1: %v", err)
	}
	// Seed a pricing agent + run.
	if _, err := st.DB.Exec(
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES('pricing','Pricing',1,?,?)`,
		fixed, fixed,
	); err != nil {
		t.Fatalf("seed pricing agent: %v", err)
	}
	if _, err := st.DB.Exec(
		`INSERT INTO runs(id, persona, trigger, status, scheduled_at, created_at) VALUES('r2','pricing','tick','failed',?,?)`,
		fixed, fixed,
	); err != nil {
		t.Fatalf("seed r2: %v", err)
	}

	resp, err := http.Get(ts.URL + "/v1/runs?persona=marketing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Runs []scheduler.Run `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Runs) != 1 || body.Runs[0].Persona != "marketing" {
		t.Errorf("got %d runs (%+v), want 1 marketing", len(body.Runs), body.Runs)
	}
}

func TestPostRuns_UnknownPersona_409(t *testing.T) {
	srv, ts, _ := newRunsTestRig(t)
	defer ts.Close()

	// Wire a scheduler so the handler doesn't 503.
	// Reuse srv.store directly to share the same in-memory DB connection.
	// Limit to 1 connection so `:memory:` SQLite doesn't spawn a fresh empty DB
	// on the scheduler's background goroutine.
	srv.store.DB.SetMaxOpenConns(1)
	sch := &scheduler.Scheduler{
		Store: srv.store,
		Now:   time.Now,
	}
	_ = sch.Start(context.Background())
	srv.SetScheduler(sch)

	body := strings.NewReader(`{"persona":"nope"}`)
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
}

func TestPostRuns_NoScheduler_503(t *testing.T) {
	_, ts, _ := newRunsTestRig(t)
	defer ts.Close()
	body := strings.NewReader(`{"persona":"marketing"}`)
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestPostRuns_EnqueuesManual(t *testing.T) {
	srv, ts, _ := newRunsTestRig(t)
	defer ts.Close()
	// Reuse srv.store directly to share the same in-memory DB connection.
	// Limit to 1 connection so `:memory:` SQLite doesn't spawn a fresh empty DB
	// on the scheduler's background goroutine.
	srv.store.DB.SetMaxOpenConns(1)
	sch := &scheduler.Scheduler{Store: srv.store, Now: time.Now}
	_ = sch.Start(context.Background())
	srv.SetScheduler(sch)

	body := strings.NewReader(`{"persona":"marketing"}`)
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var n int
	_ = srv.store.DB.QueryRow(`SELECT count(*) FROM runs WHERE persona='marketing' AND trigger='manual'`).Scan(&n)
	if n != 1 {
		t.Errorf("expected 1 manual run, got %d", n)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	_, ts, _ := newRunsTestRig(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/v1/runs/does-not-exist")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetRun_HappyPath(t *testing.T) {
	_, ts, st := newRunsTestRig(t)
	defer ts.Close()
	fixed := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.Exec(
		`INSERT INTO runs(id, persona, trigger, status, scheduled_at, created_at) VALUES('r1','marketing','tick','succeeded',?,?)`,
		fixed, fixed,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, err := http.Get(ts.URL + "/v1/runs/r1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Run        scheduler.Run   `json:"run"`
		TurnEvent  any             `json:"turn_event"`
		RetryChain []scheduler.Run `json:"retry_chain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Run.ID != "r1" {
		t.Errorf("run.id = %s, want r1", body.Run.ID)
	}
	if len(body.RetryChain) != 1 || body.RetryChain[0].ID != "r1" {
		t.Errorf("retry_chain = %+v, want single r1 entry", body.RetryChain)
	}
	if body.TurnEvent != nil {
		t.Errorf("turn_event = %v, want nil (no turn_id set on seeded row)", body.TurnEvent)
	}
}
