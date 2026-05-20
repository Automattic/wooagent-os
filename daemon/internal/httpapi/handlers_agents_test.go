package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// fakeRegistryPersona is a minimal Persona for registry-driven flag tests.
// Slug and Addable are configurable; everything else is stubbed.
type fakeRegistryPersona struct {
	slug    string
	addable bool
}

func (f *fakeRegistryPersona) Slug() string        { return f.slug }
func (f *fakeRegistryPersona) DisplayName() string { return "fake " + f.slug }
func (f *fakeRegistryPersona) Addable() bool       { return f.addable }
func (f *fakeRegistryPersona) Cooldown() personas.CooldownPolicy {
	return personas.CooldownPolicy{TargetKey: "product_id"}
}
func (f *fakeRegistryPersona) Draft(_ context.Context, _ personas.Deps) (personas.Drafted, error) {
	return personas.Drafted{Skipped: true, SkipReason: "fake"}, nil
}

// newAgentsTestRig wires a Server with the agents routes. Returns the
// test server URL and the store so tests can seed agents rows directly.
func newAgentsTestRig(t *testing.T) (string, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st}
	r := chi.NewRouter()
	r.Get("/v1/agents", s.handleListAgents)
	r.Patch("/v1/agents/{slug}", s.handlePatchAgent)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts.URL, st
}

func seedAgent(t *testing.T, st *store.Store, slug, name, model string, cadence int, enabled bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, model_preference, cadence_seconds, enabled, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		slug, name, model, cadence, enabledInt, now, now,
	)
	if err != nil {
		t.Fatalf("seed agent %s: %v", slug, err)
	}
}

func listAgents(t *testing.T, baseURL string) []Persona {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/agents")
	if err != nil {
		t.Fatalf("GET /v1/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/agents: status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Agents []Persona `json:"agents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Agents
}

func patchAgent(t *testing.T, baseURL, slug string, body any) (*http.Response, Persona) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode patch: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/agents/"+slug, &buf)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, Persona{}
	}
	var p Persona
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode patch resp: %v", err)
	}
	_ = resp.Body.Close()
	return resp, p
}

func findAgent(agents []Persona, slug string) (Persona, bool) {
	for _, a := range agents {
		if a.Persona == slug {
			return a, true
		}
	}
	return Persona{}, false
}

// ---------- GET /v1/agents ----------

func TestListAgents_AnnotatesRegistryFlags(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing", addable: false})
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "reporting", addable: true})

	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)
	// reporting is intentionally NOT seeded — it should appear via the
	// registry merge with Enabled=false, Implemented=true, Addable=true.

	agents := listAgents(t, url)

	mk, ok := findAgent(agents, "marketing")
	if !ok {
		t.Fatalf("marketing missing from list: %+v", agents)
	}
	if !mk.Implemented || mk.Addable {
		t.Errorf("marketing flags: Implemented=%v Addable=%v; want true/false", mk.Implemented, mk.Addable)
	}
	if !mk.Enabled {
		t.Errorf("marketing should be enabled (seeded that way)")
	}

	rp, ok := findAgent(agents, "reporting")
	if !ok {
		t.Fatalf("reporting missing from list (should be registry-merged): %+v", agents)
	}
	if !rp.Implemented || !rp.Addable {
		t.Errorf("reporting flags: Implemented=%v Addable=%v; want true/true", rp.Implemented, rp.Addable)
	}
	if rp.Enabled {
		t.Errorf("reporting should be disabled (never seeded)")
	}
}

func TestListAgents_HistoricalRowNoRegistry(t *testing.T) {
	// A DB row whose persona has been removed from the build still
	// surfaces — flagged as not-implemented so the UI can render it
	// honestly rather than offering controls it can't drive.
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "ghost-persona", "Ghost", "", 0, false)

	agents := listAgents(t, url)
	g, ok := findAgent(agents, "ghost-persona")
	if !ok {
		t.Fatalf("ghost-persona missing: %+v", agents)
	}
	if g.Implemented || g.Addable {
		t.Errorf("ghost flags: Implemented=%v Addable=%v; want false/false", g.Implemented, g.Addable)
	}
}

// ---------- PATCH /v1/agents/{slug} ----------

func TestPatchAgent_404Unregistered(t *testing.T) {
	url, _ := newAgentsTestRig(t)
	enabled := true
	resp, _ := patchAgent(t, url, "no-such-persona", map[string]any{"enabled": enabled})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", resp.StatusCode)
	}
}

func TestPatchAgent_BadJSON(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing"})
	url, _ := newAgentsTestRig(t)

	req, err := http.NewRequest(http.MethodPatch, url+"/v1/agents/marketing",
		bytes.NewBufferString(`{not json`))
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
}

func TestPatchAgent_FirstEnableCreatesRow(t *testing.T) {
	// reporting is addable + not yet seeded — PATCH enabled=true should
	// INSERT the row with the per-persona cadence default (86400s).
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "reporting", addable: true})
	url, st := newAgentsTestRig(t)

	enabled := true
	resp, p := patchAgent(t, url, "reporting", map[string]any{"enabled": enabled})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !p.Enabled {
		t.Errorf("response enabled=false; want true")
	}
	if !p.Implemented || !p.Addable {
		t.Errorf("registry flags missing on PATCH response: Implemented=%v Addable=%v", p.Implemented, p.Addable)
	}
	if p.CadenceSeconds != 86400 {
		t.Errorf("cadence_seconds=%d want 86400 (Reporting default)", p.CadenceSeconds)
	}

	// And the row was actually written.
	var enabledInt, cadence int
	err := st.DB.QueryRowContext(context.Background(),
		`SELECT enabled, cadence_seconds FROM agents WHERE persona = ?`, "reporting").
		Scan(&enabledInt, &cadence)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if enabledInt != 1 || cadence != 86400 {
		t.Errorf("row state: enabled=%d cadence=%d", enabledInt, cadence)
	}
}

func TestPatchAgent_Disable(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing"})
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)

	disabled := false
	resp, p := patchAgent(t, url, "marketing", map[string]any{"enabled": disabled})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if p.Enabled {
		t.Errorf("response enabled=true; want false")
	}
	// Other fields preserved.
	if p.ModelPreference != "anthropic/claude-sonnet-4-6" {
		t.Errorf("model_preference clobbered: %q", p.ModelPreference)
	}
	if p.CadenceSeconds != 21600 {
		t.Errorf("cadence_seconds clobbered: %d", p.CadenceSeconds)
	}
}

func TestPatchAgent_ModelPreferenceOnly(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing"})
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)

	resp, p := patchAgent(t, url, "marketing", map[string]any{
		"model_preference": "anthropic/claude-opus-4-7",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if p.ModelPreference != "anthropic/claude-opus-4-7" {
		t.Errorf("model_preference=%q want opus", p.ModelPreference)
	}
	if !p.Enabled {
		t.Errorf("enabled flipped unexpectedly")
	}
	if p.CadenceSeconds != 21600 {
		t.Errorf("cadence_seconds clobbered: %d", p.CadenceSeconds)
	}
}

func TestPatchAgent_CadenceOnly(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing"})
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)

	resp, p := patchAgent(t, url, "marketing", map[string]any{"cadence_seconds": 3600})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if p.CadenceSeconds != 3600 {
		t.Errorf("cadence_seconds=%d want 3600", p.CadenceSeconds)
	}
}

func TestPatchAgent_NameOnly(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing"})
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)

	resp, p := patchAgent(t, url, "marketing", map[string]any{"name": "Brand voice"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if p.Name != "Brand voice" {
		t.Errorf("name=%q want Brand voice", p.Name)
	}
	if !p.Enabled || p.ModelPreference != "anthropic/claude-sonnet-4-6" || p.CadenceSeconds != 21600 {
		t.Errorf("other fields disturbed by name patch: %+v", p)
	}
}

func TestPatchAgent_EmptyBodyTouchesUpdatedAt(t *testing.T) {
	// Empty body is treated as "touch this row." Not strictly used by the
	// UI today, but keeps the contract simple: the caller doesn't have to
	// special-case an empty patch.
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing"})
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)

	resp, p := patchAgent(t, url, "marketing", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !p.Enabled || p.ModelPreference != "anthropic/claude-sonnet-4-6" || p.CadenceSeconds != 21600 {
		t.Errorf("fields disturbed by empty patch: %+v", p)
	}
}

// ---------- PATCH /v1/agents/{slug} — apply_hours validation ----------

// patchAgentRaw issues a PATCH and returns the raw *http.Response without
// decoding, so callers can inspect non-200 error bodies.
func patchAgentRaw(t *testing.T, baseURL, slug string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode patch: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/agents/"+slug, &buf)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	return resp
}

func TestPatchAgent_HoursFormatValidation(t *testing.T) {
	personas.RegisterForTest(t, &fakeRegistryPersona{slug: "marketing", addable: false})
	url, st := newAgentsTestRig(t)
	seedAgent(t, st, "marketing", "Marketing", "anthropic/claude-sonnet-4-6", 21600, true)

	cases := []struct {
		name           string
		body           map[string]any
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:       "valid HH:MM both set",
			body:       map[string]any{"apply_hours_start": "09:00", "apply_hours_end": "17:00"},
			wantStatus: http.StatusOK,
		},
		{
			name:           "malformed start rejected",
			body:           map[string]any{"apply_hours_start": "9am", "apply_hours_end": "17:00"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "apply_hours_start",
		},
		{
			name:           "out-of-range hour rejected",
			body:           map[string]any{"apply_hours_start": "25:00", "apply_hours_end": "17:00"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "apply_hours_start",
		},
		{
			name:           "XOR rejected (only start set)",
			body:           map[string]any{"apply_hours_start": "09:00"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "both",
		},
		{
			name:           "XOR rejected (only end set)",
			body:           map[string]any{"apply_hours_end": "17:00"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "both",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := patchAgentRaw(t, url, "marketing", tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status=%d want %d body=%s", resp.StatusCode, tc.wantStatus, body)
			}
			if tc.wantBodySubstr != "" {
				body, _ := io.ReadAll(resp.Body)
				if !strings.Contains(string(body), tc.wantBodySubstr) {
					t.Errorf("response body %q does not contain %q", body, tc.wantBodySubstr)
				}
			}
		})
	}
}
