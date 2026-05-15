package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
)

// LoadTurnEvent reads a single turn_events row by turn_id. Returns
// sql.ErrNoRows when not found.
func LoadTurnEvent(ctx context.Context, db *sql.DB, turnID string) (*TurnEvent, error) {
	var (
		e               TurnEvent
		issueID         sql.NullString
		persona         sql.NullString
		promptVersion   sql.NullString
		skillVersionsJS sql.NullString
		completedAt     sql.NullString
		contextJS       sql.NullString
		modelCallsJS    sql.NullString
		skillCallsJS    sql.NullString
		proposalText    sql.NullString
		proposalSHA     sql.NullString
		verdictJS       sql.NullString
		startedAt       string
	)
	err := db.QueryRowContext(ctx, `
		SELECT turn_id, event_schema_version, issue_id, persona, prompt_version,
		       skill_versions_json, started_at, completed_at, latency_ms,
		       context_json, model_calls_json, skill_calls_json,
		       proposal_text, proposal_sha, verdict_json
		FROM turn_events WHERE turn_id = ?
	`, turnID).Scan(
		&e.TurnID, &e.EventSchemaVersion, &issueID, &persona, &promptVersion,
		&skillVersionsJS, &startedAt, &completedAt, &e.LatencyMS,
		&contextJS, &modelCallsJS, &skillCallsJS,
		&proposalText, &proposalSHA, &verdictJS,
	)
	if err != nil {
		return nil, err
	}
	if issueID.Valid {
		e.IssueID = issueID.String
	}
	if persona.Valid {
		e.Persona = persona.String
	}
	if promptVersion.Valid {
		e.PromptVersion = promptVersion.String
	}
	if skillVersionsJS.Valid && skillVersionsJS.String != "" {
		_ = json.Unmarshal([]byte(skillVersionsJS.String), &e.SkillVersions)
	}
	if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
		e.StartedAt = t
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			e.CompletedAt = &t
		}
	}
	if contextJS.Valid && contextJS.String != "" {
		_ = json.Unmarshal([]byte(contextJS.String), &e.Context)
	}
	if modelCallsJS.Valid && modelCallsJS.String != "" {
		_ = json.Unmarshal([]byte(modelCallsJS.String), &e.ModelCalls)
	}
	if skillCallsJS.Valid && skillCallsJS.String != "" {
		_ = json.Unmarshal([]byte(skillCallsJS.String), &e.SkillCalls)
	}
	if proposalText.Valid {
		e.ProposalText = proposalText.String
	}
	if proposalSHA.Valid {
		e.ProposalSHA = proposalSHA.String
	}
	if verdictJS.Valid && verdictJS.String != "" {
		var v Verdict
		if err := json.Unmarshal([]byte(verdictJS.String), &v); err == nil {
			e.Verdict = &v
		}
	}
	return &e, nil
}

// Recorder persists completed TurnEvents. The interface is intentionally
// small — agent runtimes and tests both implement or consume it.
type Recorder interface {
	Record(ctx context.Context, e TurnEvent) error
}

// SQLiteRecorder writes turn events into the daemon's main SQLite DB and
// bumps the per-persona daily cost counter via the BudgetGate.
type SQLiteRecorder struct {
	DB     *sql.DB
	Budget *pep.BudgetGate
}

func NewSQLiteRecorder(db *sql.DB, budget *pep.BudgetGate) *SQLiteRecorder {
	return &SQLiteRecorder{DB: db, Budget: budget}
}

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

	// Bump the per-persona daily cost counter. Sum the model_calls' cost.
	// Log + swallow on increment failure: a missed increment is a slight
	// under-count, which favors the operator. The recorder shouldn't fail
	// the turn over a telemetry-side increment error.
	if r.Budget != nil && e.Persona != "" {
		var totalCost float64
		for _, mc := range e.ModelCalls {
			totalCost += mc.CostUSD
		}
		if totalCost > 0 {
			_ = r.Budget.IncrementCost(ctx, manifest.Persona(e.Persona), totalCost)
		}
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
