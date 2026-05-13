package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Recorder persists completed TurnEvents. The interface is intentionally
// small — agent runtimes and tests both implement or consume it.
type Recorder interface {
	Record(ctx context.Context, e TurnEvent) error
}

// SQLiteRecorder writes turn events into the daemon's main SQLite DB. It is
// the only production impl; tests use an in-memory stub.
type SQLiteRecorder struct {
	DB *sql.DB
}

func NewSQLiteRecorder(db *sql.DB) *SQLiteRecorder { return &SQLiteRecorder{DB: db} }

func (r *SQLiteRecorder) Record(ctx context.Context, e TurnEvent) error {
	if e.TurnID == "" {
		return fmt.Errorf("turn_id is required")
	}
	if e.EventSchemaVersion == 0 {
		e.EventSchemaVersion = EventSchemaVersion
	}
	skillVersionsJSON, _ := json.Marshal(e.SkillVersions)
	contextJSON, _ := json.Marshal(e.Context)
	modelCallsJSON, _ := json.Marshal(e.ModelCalls)
	skillCallsJSON, _ := json.Marshal(e.SkillCalls)
	var verdictJSON []byte
	if e.Verdict != nil {
		verdictJSON, _ = json.Marshal(e.Verdict)
	}
	var completedAt *string
	if e.CompletedAt != nil {
		s := e.CompletedAt.UTC().Format(time.RFC3339)
		completedAt = &s
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO turn_events(
			turn_id, event_schema_version, issue_id, persona, prompt_version,
			skill_versions_json, started_at, completed_at, latency_ms,
			context_json, model_calls_json, skill_calls_json,
			proposal_text, proposal_sha, verdict_json, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.TurnID, e.EventSchemaVersion, nullIfEmpty(e.IssueID), nullIfEmpty(e.Persona), nullIfEmpty(e.PromptVersion),
		string(skillVersionsJSON), e.StartedAt.UTC().Format(time.RFC3339), completedAt, e.LatencyMS,
		string(contextJSON), string(modelCallsJSON), string(skillCallsJSON),
		nullIfEmpty(e.ProposalText), nullIfEmpty(e.ProposalSHA), nullIfEmpty(string(verdictJSON)),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert turn_event: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RecordVerdict updates the most-recent turn_events row for issueID with
// the operator's verdict. Idempotent on retry but order-dependent —
// callers should record approve / reject / dismiss exactly once per issue.
//
// We update the latest row that has no verdict yet so a subsequent action
// on the same issue (e.g., reject after edit, or restore-from-archive)
// would attach to the *next* turn rather than overwriting the prior one.
// Best-effort: callers log on error but don't fail the state change.
// DSGWOO-1236.
func RecordVerdict(
	ctx context.Context,
	db *sql.DB,
	issueID string,
	v Verdict,
) error {
	if issueID == "" {
		return fmt.Errorf("RecordVerdict: empty issue_id")
	}
	if v.DecidedAt.IsZero() {
		v.DecidedAt = time.Now().UTC()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal verdict: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE turn_events
		SET verdict_json = ?
		WHERE turn_id = (
			SELECT turn_id FROM turn_events
			WHERE issue_id = ?
			  AND (verdict_json IS NULL OR verdict_json = '')
			ORDER BY started_at DESC
			LIMIT 1
		)
	`, string(b), issueID)
	if err != nil {
		return fmt.Errorf("update verdict: %w", err)
	}
	return nil
}
