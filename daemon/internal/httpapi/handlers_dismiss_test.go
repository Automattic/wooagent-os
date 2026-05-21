package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// TestDismissIssue_RecordsTurnEventVerdict is the DSGWOO-1277 #2
// regression lock-in: handleDismissIssue must attach the operator's
// reason + comment to the originating turn_events row's verdict_json
// so the GEPA pipeline downstream pairs the proposal with its "no"
// signal. The wiring landed in 19e0a14 via telemetry.RecordVerdict;
// this test keeps it from regressing once the 30-day sweeper starts
// permanently deleting dismissed issues.
func TestDismissIssue_RecordsTurnEventVerdict(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if _, err := st.DB.Exec(
		`INSERT OR IGNORE INTO agents (persona, name, enabled, created_at, updated_at)
		 VALUES ('marketing', 'Marketing', 1, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := st.DB.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at)
		 VALUES ('iss-1', 'fixture', 'marketing', 'in_review', 'medium', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	// Seed a turn_events row tied to this issue with verdict_json NULL,
	// matching what a real persona run would produce.
	if _, err := st.DB.Exec(
		`INSERT INTO turn_events (turn_id, event_schema_version, issue_id, persona, started_at, created_at)
		 VALUES ('turn-1', 1, 'iss-1', 'marketing', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed turn_event: %v", err)
	}

	s := &Server{store: st}
	r := chi.NewRouter()
	r.Post("/v1/issues/{id}/dismiss", s.handleDismissIssue)

	body, _ := json.Marshal(map[string]string{
		"reason":  "tone_off",
		"comment": "Too florid for this product line.",
	})
	req := httptest.NewRequest("POST", "/v1/issues/iss-1/dismiss", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// Issue row updated as expected.
	var status, reason, comment string
	var dismissedAt string
	if err := st.DB.QueryRow(
		`SELECT status, dismiss_reason, COALESCE(dismiss_comment, ''), dismissed_at FROM issues WHERE id = 'iss-1'`,
	).Scan(&status, &reason, &comment, &dismissedAt); err != nil {
		t.Fatalf("read issue: %v", err)
	}
	if status != "dismissed" {
		t.Errorf("status = %q, want dismissed", status)
	}
	if reason != "tone_off" {
		t.Errorf("reason = %q, want tone_off", reason)
	}
	if comment == "" {
		t.Errorf("comment was empty, expected the operator note")
	}
	if dismissedAt == "" {
		t.Errorf("dismissed_at not stamped")
	}

	// And the load-bearing piece for GEPA: verdict_json attached to
	// the turn that produced the proposal.
	var verdictJSON string
	if err := st.DB.QueryRow(
		`SELECT verdict_json FROM turn_events WHERE turn_id = 'turn-1'`,
	).Scan(&verdictJSON); err != nil {
		t.Fatalf("read turn_event: %v", err)
	}
	if verdictJSON == "" {
		t.Fatalf("verdict_json was empty; dismiss did not record a turn-event verdict")
	}
	var v telemetry.Verdict
	if err := json.Unmarshal([]byte(verdictJSON), &v); err != nil {
		t.Fatalf("unmarshal verdict: %v (raw: %s)", err, verdictJSON)
	}
	if v.Kind != telemetry.VerdictDismiss {
		t.Errorf("kind = %q, want %q", v.Kind, telemetry.VerdictDismiss)
	}
	if v.ReasonTag != "tone_off" {
		t.Errorf("reason_tag = %q, want tone_off", v.ReasonTag)
	}
	if v.ReasonText == "" {
		t.Errorf("reason_text empty, expected operator comment to flow through")
	}
	if v.DecidedAt.IsZero() {
		t.Errorf("decided_at not stamped")
	}
}
