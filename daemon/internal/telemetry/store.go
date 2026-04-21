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
