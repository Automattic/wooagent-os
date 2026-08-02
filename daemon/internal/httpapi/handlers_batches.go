package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// Batch is the v0.1 batch wire shape. Counters (Total / Pending / Approved /
// Rejected) are derived from joined children at read time so they can never
// drift out of sync with the children's statuses. See
// daemon-batch-issues-plan.md for the lightweight-grouping rationale.
type Batch struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Persona     string    `json:"persona,omitempty"`
	Intent      string    `json:"intent,omitempty"`
	SourceRunID string    `json:"source_run_id,omitempty"`
	Total       int       `json:"total"`
	Pending     int       `json:"pending"`
	Approved    int       `json:"approved"`
	Rejected    int       `json:"rejected"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IssueWithProposal pairs an issue with its proposal — the shape returned by
// GET /v1/batches/:id so the UI can render the batch review page in a single
// fetch instead of N+1 lookups.
type IssueWithProposal struct {
	Issue    Issue     `json:"issue"`
	Proposal *Proposal `json:"proposal"`
}

type createBatchChildIssue struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Persona     string    `json:"persona,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	Status      string    `json:"status,omitempty"`
	Proposal    *Proposal `json:"proposal,omitempty"`
}

type createBatchReq struct {
	Title       string                  `json:"title"`
	Persona     string                  `json:"persona"`
	Intent      string                  `json:"intent,omitempty"`
	SourceRunID string                  `json:"source_run_id,omitempty"`
	Issues      []createBatchChildIssue `json:"issues"`
}

func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var req createBatchReq
	if !decodeJSONBody(w, r, &req, "bad_json") {
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}
	if len(req.Issues) == 0 {
		writeError(w, http.StatusBadRequest, "missing_issues", "issues array is required")
		return
	}

	ctx := r.Context()

	// First cross-table app-level transaction in the daemon — see
	// daemon-batch-issues-plan.md §"Storage layer" for the rationale.
	// Defer Rollback no-ops post-commit.
	tx, err := s.store.DB.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	batchID := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO batches(id, title, persona, intent, source_run_id, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		batchID, req.Title, nullIfEmpty(req.Persona), nullIfEmpty(req.Intent), nullIfEmpty(req.SourceRunID), now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	children := make([]IssueWithProposal, 0, len(req.Issues))
	ts, _ := time.Parse(time.RFC3339, now)

	// Resolved once so every child in the batch records the same store
	// provenance (DSGWOO-1371).
	storeID := store.CurrentStoreID(ctx, s.store.DB)

	pending, approved, rejected := 0, 0, 0
	for _, child := range req.Issues {
		priority := child.Priority
		if priority == "" {
			priority = "medium"
		}
		// Batch children land in_review by default — the operator's job on
		// the batch screen is to approve/reject, not to backlog-shuffle.
		status := child.Status
		if status == "" {
			status = "in_review"
		}
		// Children inherit persona from the parent batch unless they
		// explicitly override.
		persona := child.Persona
		if persona == "" {
			persona = req.Persona
		}

		var (
			proposalType    any
			proposalContent any
			proposalTarget  any
		)
		if child.Proposal != nil {
			if child.Proposal.Type == "" {
				writeError(w, http.StatusBadRequest, "missing_proposal_type", "proposal.type is required when proposal is set")
				return
			}
			proposalType = child.Proposal.Type
			proposalContent = child.Proposal.Content
			if len(child.Proposal.Target) > 0 {
				b, err := json.Marshal(child.Proposal.Target)
				if err != nil {
					writeError(w, http.StatusBadRequest, "bad_proposal_target", err.Error())
					return
				}
				proposalTarget = string(b)
			}
		}

		childID := uuid.NewString()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target, batch_id, store_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			childID, child.Title, child.Description, nullIfEmpty(persona), status, priority, now, now,
			proposalType, proposalContent, proposalTarget, batchID,
			nullIfEmpty(storeID),
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		children = append(children, IssueWithProposal{
			Issue: Issue{
				ID: childID, Title: child.Title, Description: child.Description, Persona: persona,
				Status: status, Priority: priority, BatchID: batchID,
				CreatedAt: ts, UpdatedAt: ts,
			},
			Proposal: child.Proposal,
		})
		switch status {
		case "in_review":
			pending++
		case "done":
			approved++
		case "rejected":
			rejected++
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"batch": Batch{
			ID: batchID, Title: req.Title, Persona: req.Persona, Intent: req.Intent,
			SourceRunID: req.SourceRunID,
			Total:       len(children),
			Pending:     pending,
			Approved:    approved,
			Rejected:    rejected,
			CreatedAt:   ts,
			UpdatedAt:   ts,
		},
		"issues": children,
	})
}

func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	persona := r.URL.Query().Get("persona")

	// One round-trip — counts derived via LEFT JOIN so empty batches still
	// appear with zeros. COUNT(CASE WHEN ... THEN 1 END) over no ELSE — the
	// NULL fallthrough is what we want (NULL doesn't count).
	q := `SELECT b.id, b.title, COALESCE(b.persona, ''), COALESCE(b.intent, ''), COALESCE(b.source_run_id, ''),
                 b.created_at, b.updated_at,
                 COUNT(i.id) AS total,
                 COUNT(CASE WHEN i.status = 'in_review' THEN 1 END) AS pending,
                 COUNT(CASE WHEN i.status = 'done'      THEN 1 END) AS approved,
                 COUNT(CASE WHEN i.status = 'rejected'  THEN 1 END) AS rejected
          FROM batches b LEFT JOIN issues i ON i.batch_id = b.id`
	args := []any{}
	where := []string{}
	if persona != "" {
		where = append(where, "b.persona = ?")
		args = append(args, persona)
	}
	if len(where) > 0 {
		q += " WHERE " + joinAnd(where)
	}
	q += " GROUP BY b.id ORDER BY b.created_at DESC LIMIT 200"

	rows, err := s.store.DB.QueryContext(ctx, q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	batches := []Batch{}
	for rows.Next() {
		var b Batch
		var createdAt, updatedAt string
		if err := rows.Scan(
			&b.ID, &b.Title, &b.Persona, &b.Intent, &b.SourceRunID,
			&createdAt, &updatedAt,
			&b.Total, &b.Pending, &b.Approved, &b.Rejected,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		b.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		batches = append(batches, b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": batches})
}

func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var b Batch
	var createdAt, updatedAt string
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT b.id, b.title, COALESCE(b.persona, ''), COALESCE(b.intent, ''), COALESCE(b.source_run_id, ''),
                b.created_at, b.updated_at,
                COUNT(i.id) AS total,
                COUNT(CASE WHEN i.status = 'in_review' THEN 1 END) AS pending,
                COUNT(CASE WHEN i.status = 'done'      THEN 1 END) AS approved,
                COUNT(CASE WHEN i.status = 'rejected'  THEN 1 END) AS rejected
         FROM batches b LEFT JOIN issues i ON i.batch_id = b.id
         WHERE b.id = ?
         GROUP BY b.id`, id,
	).Scan(
		&b.ID, &b.Title, &b.Persona, &b.Intent, &b.SourceRunID,
		&createdAt, &updatedAt,
		&b.Total, &b.Pending, &b.Approved, &b.Rejected,
	)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "no batch with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	b.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	rows, err := s.store.DB.QueryContext(ctx,
		`SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, COALESCE(batch_id, ''), created_at, updated_at, proposal_type, proposal_content, proposal_target FROM issues WHERE batch_id = ? ORDER BY created_at ASC`, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	issues := []IssueWithProposal{}
	for rows.Next() {
		var i Issue
		var cAt, uAt string
		var pType, pContent, pTarget sql.NullString
		if err := rows.Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &i.BatchID, &cAt, &uAt, &pType, &pContent, &pTarget); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		i.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		i.UpdatedAt, _ = time.Parse(time.RFC3339, uAt)
		var proposal *Proposal
		if pType.Valid && pType.String != "" {
			proposal = &Proposal{Type: pType.String, Content: pContent.String}
			if pTarget.Valid && pTarget.String != "" {
				_ = json.Unmarshal([]byte(pTarget.String), &proposal.Target)
			}
		}
		issues = append(issues, IssueWithProposal{Issue: i, Proposal: proposal})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"batch":  b,
		"issues": issues,
	})
}

type approveBatchChildReq struct {
	IssueID   string `json:"issue_id"`
	VariantID string `json:"variant_id,omitempty"`
}

type approveBatchReq struct {
	Children []approveBatchChildReq `json:"children"`
}

// handleApproveBatch loops approveOne per child, returning a per-child
// {ok, status?|error?} entry. Always 200 — best-effort semantics. Sequential,
// not parallel: with N=7 children, SQLite writer-lock + audit attribution
// makes goroutines a net loss.
func (s *Server) handleApproveBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "id")

	// Verify the batch exists up front so a missing id 404s cleanly instead
	// of returning a 200 with N "not_found" per-child errors.
	var batchExists int
	err := s.store.DB.QueryRowContext(ctx, `SELECT 1 FROM batches WHERE id = ?`, batchID).Scan(&batchExists)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "no batch with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	var req approveBatchReq
	if r.ContentLength > 0 {
		if !decodeJSONBody(w, r, &req, "bad_json") {
			return
		}
	}
	if len(req.Children) == 0 {
		writeError(w, http.StatusBadRequest, "missing_children", "children array is required")
		return
	}

	results := make([]map[string]any, 0, len(req.Children))
	for _, child := range req.Children {
		if child.IssueID == "" {
			results = append(results, map[string]any{
				"issue_id": "",
				"ok":       false,
				"error":    map[string]any{"code": "missing_issue_id", "message": "issue_id is required"},
			})
			continue
		}
		// Confirm the child belongs to this batch — prevents a
		// confused-deputy-style call where the operator passes the wrong
		// batch_id and approves an unrelated issue.
		var ownedBy sql.NullString
		err := s.store.DB.QueryRowContext(ctx, `SELECT batch_id FROM issues WHERE id = ?`, child.IssueID).Scan(&ownedBy)
		if err == sql.ErrNoRows {
			results = append(results, map[string]any{
				"issue_id": child.IssueID,
				"ok":       false,
				"error":    map[string]any{"code": "not_found", "message": "no issue with that id"},
			})
			continue
		}
		if err != nil {
			results = append(results, map[string]any{
				"issue_id": child.IssueID,
				"ok":       false,
				"error":    map[string]any{"code": "db_error", "message": err.Error()},
			})
			continue
		}
		if !ownedBy.Valid || ownedBy.String != batchID {
			results = append(results, map[string]any{
				"issue_id": child.IssueID,
				"ok":       false,
				"error":    map[string]any{"code": "not_in_batch", "message": "issue does not belong to this batch"},
			})
			continue
		}

		res, perr := s.approveOne(ctx, child.IssueID, child.VariantID)
		if perr != nil {
			results = append(results, map[string]any{
				"issue_id": child.IssueID,
				"ok":       false,
				"error":    map[string]any{"code": perr.Code, "message": perr.Message},
			})
			continue
		}
		results = append(results, map[string]any{
			"issue_id":   res.IssueID,
			"ok":         true,
			"status":     res.Status,
			"ability":    res.Ability,
			"audit_id":   res.AuditID,
			"updated_at": res.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// handleRejectBatch flips every in_review child to rejected in a single
// bulk UPDATE; children that aren't in_review (already done/rejected) come
// back as ok=false with code=wrong_status rather than failing the whole
// batch. Always 200 — same best-effort shape as approve-all.
//
// Optional body: {"reason": "...", "comment": "..."}. When supplied (the UI
// always sends one via the DismissDialog flow), the reason/comment are
// persisted per child via dismiss_reason / dismiss_comment / dismissed_at —
// mirroring single-issue dismiss so the Archive screen can render the same
// chip + tooltip for batch-dismissed children. An empty body keeps the
// pre-DSGWOO-1282 behavior so scripted callers don't break.
func (s *Server) handleRejectBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "id")

	var req struct {
		Reason  string `json:"reason"`
		Comment string `json:"comment,omitempty"`
	}
	if r.ContentLength > 0 {
		if !decodeJSONBody(w, r, &req, "bad_body") {
			return
		}
	}

	var batchExists int
	err := s.store.DB.QueryRowContext(ctx, `SELECT 1 FROM batches WHERE id = ?`, batchID).Scan(&batchExists)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "no batch with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Read the children's current statuses first so the per-child results
	// can name everyone — not just the ones we're flipping.
	type childRow struct {
		id     string
		status string
	}
	rows, err := s.store.DB.QueryContext(ctx,
		`SELECT id, status FROM issues WHERE batch_id = ? ORDER BY created_at ASC`, batchID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	var children []childRow
	for rows.Next() {
		var c childRow
		if err := rows.Scan(&c.id, &c.status); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		children = append(children, c)
	}
	rows.Close()

	tx, err := s.store.DB.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	if req.Reason != "" {
		var commentArg any
		if req.Comment != "" {
			commentArg = req.Comment
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE issues
			 SET status = 'rejected',
			     dismiss_reason = ?,
			     dismiss_comment = ?,
			     dismissed_at = ?,
			     updated_at = ?
			 WHERE batch_id = ? AND status = 'in_review'`,
			req.Reason, commentArg, now, now, batchID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE issues SET status = 'rejected', updated_at = ? WHERE batch_id = ? AND status = 'in_review'`,
			now, batchID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	results := make([]map[string]any, 0, len(children))
	for _, c := range children {
		if c.status == "in_review" {
			results = append(results, map[string]any{
				"issue_id":   c.id,
				"ok":         true,
				"status":     "rejected",
				"updated_at": now,
			})
		} else {
			results = append(results, map[string]any{
				"issue_id": c.id,
				"ok":       false,
				"error":    map[string]any{"code": "wrong_status", "message": "reject requires status=in_review, found " + c.status},
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
