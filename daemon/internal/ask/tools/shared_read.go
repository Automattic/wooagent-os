// Package tools is the toolbelt for the Ask Agent drawer (DSGWOO-1348).
// Each tool wraps existing daemon storage or scheduler functionality in
// the anthropic.ToolHandler interface so the LLM can call it during a
// chat turn. Tools are agent-agnostic: the same read tools serve Chief
// of Staff today and will serve specialists in task A2.
package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
)

// stateToStatuses maps the CoS-prompt vocabulary onto the daemon's
// canonical Issue.status values. "pending" / "approved" / "rejected"
// are what the model sees; "in_review" / "done" / ("rejected" |
// "dismissed") are what's in the database.
func stateToStatuses(state string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "pending":
		return []string{"in_review"}, nil
	case "approved":
		return []string{"done"}, nil
	case "rejected":
		return []string{"rejected", "dismissed"}, nil
	default:
		return nil, fmt.Errorf("unknown state %q (expected: pending, approved, rejected)", state)
	}
}

// parseSince accepts an empty string, an ISO8601 timestamp, or a
// duration shorthand like "-7d" / "-24h". Returns the zero time if the
// input is empty (caller should skip the filter when zero).
func parseSince(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	// Duration shorthand: "-7d", "-24h", etc.
	if strings.HasPrefix(raw, "-") {
		if d, err := parseDurationShorthand(raw[1:]); err == nil {
			return time.Now().Add(-d), nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable since=%q (use ISO8601 or shorthand like -7d / -24h)", raw)
}

// parseDurationShorthand accepts "7d", "24h", "30m" and converts to
// time.Duration. Go's time.ParseDuration handles "h"/"m"/"s" but not
// "d"; this thin wrapper splits the unit out.
func parseDurationShorthand(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h")
		if err != nil {
			return 0, err
		}
		return n * 24, nil
	}
	return time.ParseDuration(s)
}

// ---------------------------------------------------------- list_proposals

// ListProposalsTool wraps a paged Issues query for the chat layer.
type ListProposalsTool struct {
	DB *sql.DB
}

type listProposalsInput struct {
	State   string `json:"state,omitempty"`
	Persona string `json:"persona,omitempty"`
	Since   string `json:"since,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type proposalSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Persona    string `json:"persona"`
	State      string `json:"state"`
	AgeSeconds int64  `json:"age_seconds"`
	CreatedAt  string `json:"created_at"`
}

func (h *ListProposalsTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "list_proposals",
		Description: "List proposals (Issues) in the operator's queue. " +
			"`state` filters by status (default 'pending'). `persona` " +
			"filters by which agent produced it. `since` is an ISO8601 " +
			"timestamp or shorthand like '-7d' / '-24h'. `limit` defaults " +
			"to 20.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"state":   { "type": "string", "enum": ["pending", "approved", "rejected"], "default": "pending" },
				"persona": { "type": "string" },
				"since":   { "type": "string", "description": "ISO8601 or shorthand like -7d" },
				"limit":   { "type": "integer", "default": 20, "minimum": 1, "maximum": 100 }
			}
		}`),
	}
}

func (h *ListProposalsTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in listProposalsInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}
	statuses, err := stateToStatuses(in.State)
	if err != nil {
		return "", err
	}
	since, err := parseSince(in.Since)
	if err != nil {
		return "", err
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 100 {
		in.Limit = 100
	}

	// Build the query with dynamic placeholders for the status list.
	placeholders := strings.Repeat("?,", len(statuses))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(statuses)+2)
	for _, s := range statuses {
		args = append(args, s)
	}
	q := `SELECT id, title, COALESCE(persona, ''), status, created_at
	      FROM issues WHERE status IN (` + placeholders + `)`
	if in.Persona != "" {
		q += ` AND persona = ?`
		args = append(args, in.Persona)
	}
	if !since.IsZero() {
		q += ` AND created_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, in.Limit)

	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	out := struct {
		Proposals []proposalSummary `json:"proposals"`
		Count     int               `json:"count"`
		State     string            `json:"state_used"`
	}{State: orPending(in.State)}
	for rows.Next() {
		var p proposalSummary
		var rawStatus, createdAt string
		if err := rows.Scan(&p.ID, &p.Title, &p.Persona, &rawStatus, &createdAt); err != nil {
			return "", fmt.Errorf("scan: %w", err)
		}
		p.State = statusToState(rawStatus)
		p.CreatedAt = createdAt
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			p.AgeSeconds = int64(now.Sub(t).Seconds())
		}
		out.Proposals = append(out.Proposals, p)
	}
	out.Count = len(out.Proposals)
	return marshalJSON(out)
}

// statusToState is the reverse of stateToStatuses for one specific
// status, used when surfacing rows back to the model in chat vocab.
func statusToState(daemonStatus string) string {
	switch daemonStatus {
	case "in_review":
		return "pending"
	case "done":
		return "approved"
	case "rejected", "dismissed":
		return "rejected"
	default:
		return daemonStatus
	}
}

func orPending(s string) string {
	if s == "" {
		return "pending"
	}
	return s
}

// ------------------------------------------------------------ get_proposal

// GetProposalTool returns the full record for one Issue, including its
// rejection note (if any) and the latest run linked to it.
type GetProposalTool struct {
	DB *sql.DB
}

type getProposalInput struct {
	ID string `json:"id"`
}

type proposalDetail struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Persona        string `json:"persona"`
	State          string `json:"state"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	AgeSeconds     int64  `json:"age_seconds"`
	DismissReason  string `json:"dismiss_reason,omitempty"`
	DismissComment string `json:"dismiss_comment,omitempty"`
	DismissedAt    string `json:"dismissed_at,omitempty"`
	LatestRunID    string `json:"latest_run_id,omitempty"`
	LatestRunState string `json:"latest_run_state,omitempty"`
}

func (h *GetProposalTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name:        "get_proposal",
		Description: "Fetch the full record for one proposal by id, including rejection notes (if any) and the latest linked run.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": { "id": { "type": "string" } },
			"required": ["id"]
		}`),
	}
}

func (h *GetProposalTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in getProposalInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.ID) == "" {
		return "", errors.New("id is required")
	}

	var (
		d              proposalDetail
		rawStatus      string
		createdAt      string
		updatedAt      string
		dismissReason  sql.NullString
		dismissComment sql.NullString
		dismissedAt    sql.NullString
	)
	err := h.DB.QueryRowContext(ctx,
		`SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status,
		        created_at, updated_at, dismiss_reason, dismiss_comment, dismissed_at
		   FROM issues WHERE id = ?`, in.ID).Scan(
		&d.ID, &d.Title, &d.Description, &d.Persona, &rawStatus,
		&createdAt, &updatedAt, &dismissReason, &dismissComment, &dismissedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no proposal with id %q", in.ID)
	}
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	d.State = statusToState(rawStatus)
	d.CreatedAt = createdAt
	d.UpdatedAt = updatedAt
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		d.AgeSeconds = int64(time.Since(t).Seconds())
	}
	if dismissReason.Valid {
		d.DismissReason = dismissReason.String
	}
	if dismissComment.Valid {
		d.DismissComment = dismissComment.String
	}
	if dismissedAt.Valid {
		d.DismissedAt = dismissedAt.String
	}

	// Look up the latest run linked to this issue. Many issues won't
	// have one (manual creations) — that's fine; field stays empty.
	var (
		latestRunID    sql.NullString
		latestRunState sql.NullString
	)
	_ = h.DB.QueryRowContext(ctx,
		`SELECT id, status FROM runs WHERE issue_id = ? ORDER BY created_at DESC LIMIT 1`,
		in.ID).Scan(&latestRunID, &latestRunState)
	if latestRunID.Valid {
		d.LatestRunID = latestRunID.String
	}
	if latestRunState.Valid {
		d.LatestRunState = latestRunState.String
	}

	return marshalJSON(d)
}

// ---------------------------------------------------------------- list_runs

// ListRunsTool returns a recent slice of scheduler Runs, summarized.
type ListRunsTool struct {
	DB *sql.DB
}

type listRunsInput struct {
	Persona string `json:"persona,omitempty"`
	Since   string `json:"since,omitempty"`
	Status  string `json:"status,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type runSummary struct {
	ID      string `json:"id"`
	Persona string `json:"persona"`
	Trigger string `json:"trigger"`
	Status  string `json:"status"`
	IssueID string `json:"issue_id,omitempty"`
	// IssueTitle / IssueState carry the linked proposal's metadata so
	// the model can cite the proposal (the operator-facing artifact)
	// instead of the run when reporting on completed work. Empty when
	// the run hasn't produced a proposal (queued, running, failed) or
	// when the join missed (proposal deleted).
	IssueTitle    string `json:"issue_title,omitempty"`
	IssueState    string `json:"issue_state,omitempty"`
	CreatedAt     string `json:"created_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
	LatencyMS     int64  `json:"latency_ms,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

func (h *ListRunsTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name:        "list_runs",
		Description: "List recent scheduler runs. `persona` filters to one agent; `status` filters (e.g. 'succeeded', 'failed', 'queued', 'running'); `since` is an ISO8601 timestamp or shorthand like '-24h'. `limit` defaults to 20.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"persona": { "type": "string" },
				"since":   { "type": "string" },
				"status":  { "type": "string" },
				"limit":   { "type": "integer", "default": 20, "minimum": 1, "maximum": 100 }
			}
		}`),
	}
}

func (h *ListRunsTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in listRunsInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}
	since, err := parseSince(in.Since)
	if err != nil {
		return "", err
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 100 {
		in.Limit = 100
	}

	// LEFT JOIN issues so each row carries the linked proposal's title
	// + status (chat-vocab "state" is mapped post-scan). Lets the
	// reference layer index runs as proposals; see DSGWOO-1362.
	q := `SELECT r.id, r.persona, r.trigger, r.status, COALESCE(r.issue_id, ''),
	             r.created_at, COALESCE(r.completed_at, ''),
	             COALESCE(r.latency_ms, 0), COALESCE(r.failure_reason, ''),
	             COALESCE(i.title, ''), COALESCE(i.status, '')
	      FROM runs r
	      LEFT JOIN issues i ON r.issue_id = i.id
	      WHERE 1=1`
	args := make([]any, 0, 4)
	if in.Persona != "" {
		q += ` AND r.persona = ?`
		args = append(args, in.Persona)
	}
	if in.Status != "" {
		q += ` AND r.status = ?`
		args = append(args, in.Status)
	}
	if !since.IsZero() {
		q += ` AND r.created_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY r.created_at DESC LIMIT ?`
	args = append(args, in.Limit)

	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	out := struct {
		Runs  []runSummary `json:"runs"`
		Count int          `json:"count"`
	}{}
	for rows.Next() {
		var r runSummary
		var issueRawStatus string
		if err := rows.Scan(&r.ID, &r.Persona, &r.Trigger, &r.Status, &r.IssueID,
			&r.CreatedAt, &r.CompletedAt, &r.LatencyMS, &r.FailureReason,
			&r.IssueTitle, &issueRawStatus); err != nil {
			return "", fmt.Errorf("scan: %w", err)
		}
		if issueRawStatus != "" {
			r.IssueState = statusToState(issueRawStatus)
		}
		out.Runs = append(out.Runs, r)
	}
	out.Count = len(out.Runs)
	return marshalJSON(out)
}

// ----------------------------------------------------------------- get_run

// GetRunTool returns the full record for one Run.
type GetRunTool struct {
	DB *sql.DB
}

type getRunInput struct {
	ID string `json:"id"`
}

func (h *GetRunTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name:        "get_run",
		Description: "Fetch the full record for one run by id, including timing, status, and any failure reason.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": { "id": { "type": "string" } },
			"required": ["id"]
		}`),
	}
}

func (h *GetRunTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in getRunInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.ID) == "" {
		return "", errors.New("id is required")
	}

	var r runSummary
	err := h.DB.QueryRowContext(ctx,
		`SELECT id, persona, trigger, status, COALESCE(issue_id, ''),
		        created_at, COALESCE(completed_at, ''),
		        COALESCE(latency_ms, 0), COALESCE(failure_reason, '')
		   FROM runs WHERE id = ?`, in.ID).Scan(
		&r.ID, &r.Persona, &r.Trigger, &r.Status, &r.IssueID,
		&r.CreatedAt, &r.CompletedAt, &r.LatencyMS, &r.FailureReason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no run with id %q", in.ID)
	}
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	return marshalJSON(r)
}

// -------------------------------------------------------------- list_agents

// ListAgentsTool returns the registered personas + their enable state
// and last-run timestamp.
type ListAgentsTool struct {
	DB *sql.DB
}

type agentSummary struct {
	Persona        string `json:"persona"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Implemented    bool   `json:"implemented"`
	CadenceSeconds int    `json:"cadence_seconds,omitempty"`
	LastRunAt      string `json:"last_run_at,omitempty"`
}

func (h *ListAgentsTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name:        "list_agents",
		Description: "List all known personas with their enable state, cadence, and most recent run timestamp.",
		InputSchema: json.RawMessage(`{ "type": "object" }`),
	}
}

func (h *ListAgentsTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	dbEnabled, dbCadence, dbLastRun, err := loadAgentRows(ctx, h.DB)
	if err != nil {
		return "", fmt.Errorf("agents query: %w", err)
	}

	seen := map[string]bool{}
	out := struct {
		Agents []agentSummary `json:"agents"`
	}{}

	// Runtime-registered personas first (canonical source).
	for _, p := range personas.All() {
		slug := p.Slug()
		seen[slug] = true
		out.Agents = append(out.Agents, agentSummary{
			Persona:        slug,
			Name:           p.DisplayName(),
			Enabled:        dbEnabled[slug],
			Implemented:    true,
			CadenceSeconds: dbCadence[slug],
			LastRunAt:      dbLastRun[slug],
		})
	}
	// Then DB-only rows (configured but not registered — should be rare
	// after the persona-add/disable refactor).
	for slug, enabled := range dbEnabled {
		if seen[slug] {
			continue
		}
		out.Agents = append(out.Agents, agentSummary{
			Persona:        slug,
			Name:           slug,
			Enabled:        enabled,
			Implemented:    false,
			CadenceSeconds: dbCadence[slug],
			LastRunAt:      dbLastRun[slug],
		})
	}
	return marshalJSON(out)
}

// loadAgentRows pulls per-persona configuration from the agents table
// and the most recent run timestamp from the runs table. Empty maps are
// returned when the tables are empty or missing — list_agents falls back
// on the runtime registry in either case.
func loadAgentRows(ctx context.Context, db *sql.DB) (enabled map[string]bool, cadence map[string]int, lastRun map[string]string, err error) {
	enabled = map[string]bool{}
	cadence = map[string]int{}
	lastRun = map[string]string{}

	rows, err := db.QueryContext(ctx,
		`SELECT persona, COALESCE(enabled, 0), COALESCE(cadence_seconds, 0) FROM agents`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var en int
		var cad int
		if err := rows.Scan(&slug, &en, &cad); err != nil {
			return nil, nil, nil, err
		}
		enabled[slug] = en == 1
		cadence[slug] = cad
	}

	runRows, err := db.QueryContext(ctx,
		`SELECT persona, MAX(created_at) FROM runs GROUP BY persona`)
	if err != nil {
		return enabled, cadence, lastRun, nil // soft-fail — runs may be empty
	}
	defer runRows.Close()
	for runRows.Next() {
		var slug string
		var ts sql.NullString
		if err := runRows.Scan(&slug, &ts); err != nil {
			continue
		}
		if ts.Valid {
			lastRun[slug] = ts.String
		}
	}
	return enabled, cadence, lastRun, nil
}

// marshalJSON is a thin wrapper that returns the string form of a JSON
// payload — the shape Anthropic's tool_result expects.
func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b), nil
}

// Compile-time assertions that each tool satisfies the ToolHandler
// interface; cheaper than failing at the call site.
var (
	_ anthropic.ToolHandler = (*ListProposalsTool)(nil)
	_ anthropic.ToolHandler = (*GetProposalTool)(nil)
	_ anthropic.ToolHandler = (*ListRunsTool)(nil)
	_ anthropic.ToolHandler = (*GetRunTool)(nil)
	_ anthropic.ToolHandler = (*ListAgentsTool)(nil)
)

// Type-check that scheduler.Trigger is the type our DispatchTool
// references; pulled in here to keep imports honest.
var _ scheduler.Trigger = scheduler.TriggerOperatorAsked
