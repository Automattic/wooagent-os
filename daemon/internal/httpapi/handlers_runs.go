package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// handleListRuns: GET /v1/runs?persona=&status=&issue_id=&limit=&cursor=
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var args []any
	where := []string{"1=1"}
	if v := q.Get("persona"); v != "" {
		where = append(where, "persona = ?")
		args = append(args, v)
	}
	if v := q.Get("status"); v != "" {
		parts := strings.Split(v, ",")
		placeholders := strings.Repeat("?,", len(parts))
		placeholders = strings.TrimSuffix(placeholders, ",")
		where = append(where, "status IN ("+placeholders+")")
		for _, p := range parts {
			args = append(args, p)
		}
	}
	if v := q.Get("issue_id"); v != "" {
		where = append(where, "issue_id = ?")
		args = append(args, v)
	}
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := q.Get("cursor"); v != "" {
		// Cursor pagination by (created_at, id) ordering. Decode lazily —
		// accept the run id and compute the where clause via subquery.
		where = append(where, "(created_at, id) < (SELECT created_at, id FROM runs WHERE id = ?)")
		args = append(args, v)
	}
	query := fmt.Sprintf(`
		SELECT id, persona, trigger, status, attempt, retry_of,
		       scheduled_at, claimed_at, completed_at, latency_ms,
		       issue_id, turn_id, skip_reason, failure_reason, failure_class, created_at
		FROM runs WHERE %s ORDER BY created_at DESC, id DESC LIMIT ?
	`, strings.Join(where, " AND "))
	args = append(args, limit+1)

	rows, err := s.store.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	runs := []scheduler.Run{}
	for rows.Next() {
		run, err := scheduler.ScanRunForRow(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scan", err.Error())
			return
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "iterate", err.Error())
		return
	}
	var nextCursor *string
	if len(runs) > limit {
		last := runs[limit-1]
		nextCursor = &last.ID
		runs = runs[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runs":        runs,
		"next_cursor": nextCursor,
	})
}

// handleGetRun: GET /v1/runs/:id
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	run, err := loadRun(r.Context(), s.store.DB, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "run_not_found", "no run with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	var turnEvent *telemetry.TurnEvent
	if run.TurnID != nil {
		te, err := telemetry.LoadTurnEvent(r.Context(), s.store.DB, *run.TurnID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		turnEvent = te
	}
	chain, err := loadRetryChain(r.Context(), s.store.DB, run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run":         run,
		"turn_event":  turnEvent,
		"retry_chain": chain,
	})
}

func loadRun(ctx context.Context, db *sql.DB, id string) (*scheduler.Run, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, persona, trigger, status, attempt, retry_of,
		       scheduled_at, claimed_at, completed_at, latency_ms,
		       issue_id, turn_id, skip_reason, failure_reason, failure_class, created_at
		FROM runs WHERE id=?
	`, id)
	return scheduler.ScanRunForRow(row)
}

func loadRetryChain(ctx context.Context, db *sql.DB, r *scheduler.Run) ([]scheduler.Run, error) {
	// Walk up to root.
	cur := r
	root := cur.ID
	for cur.RetryOf != nil {
		parent, err := loadRun(ctx, db, *cur.RetryOf)
		if err != nil {
			return nil, err
		}
		cur = parent
		root = parent.ID
	}
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE chain AS (
			SELECT * FROM runs WHERE id = ?
			UNION ALL
			SELECT r.* FROM runs r JOIN chain c ON r.retry_of = c.id
		)
		SELECT id, persona, trigger, status, attempt, retry_of,
		       scheduled_at, claimed_at, completed_at, latency_ms,
		       issue_id, turn_id, skip_reason, failure_reason, failure_class, created_at
		FROM chain ORDER BY attempt ASC
	`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scheduler.Run
	for rows.Next() {
		run, err := scheduler.ScanRunForRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// handleCancelRun: POST /v1/runs/:id/cancel
//
// Marks a queued or running row as failed_permanent so a stuck run no
// longer blocks the persona's Loop.hasActiveRun gate. 404 if the row
// doesn't exist; 409 if it's already terminal. Body: none.
func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler_unavailable", "scheduler not running")
		return
	}
	id := chi.URLParam(r, "id")
	run, err := s.scheduler.CancelRun(r.Context(), id)
	if errors.Is(err, scheduler.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "run_not_found", "no run with that id")
		return
	}
	if errors.Is(err, scheduler.ErrRunNotCancellable) {
		writeError(w, http.StatusConflict, "run_not_cancellable", "run is already in a terminal state")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cancel_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

// handleCreateRun: POST /v1/runs body: { "persona": "marketing" }
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler_unavailable", "scheduler not running")
		return
	}
	var body struct {
		Persona string `json:"persona"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", "decode body: "+err.Error())
		return
	}
	if body.Persona == "" {
		writeError(w, http.StatusBadRequest, "missing_persona", "persona is required")
		return
	}
	run, err := s.scheduler.EnqueueManual(r.Context(), body.Persona)
	if errors.Is(err, scheduler.ErrPersonaUnavailable) {
		writeError(w, http.StatusConflict, "persona_disabled", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "enqueue_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"run": run})
}
