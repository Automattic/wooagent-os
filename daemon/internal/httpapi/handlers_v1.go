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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
)

// Persona is the v0.1 agent-persona wire shape. Mirrors docs/api-contract-v1.md.
type Persona struct {
	Persona         string `json:"persona"`
	Name            string `json:"name"`
	ModelPreference string `json:"model_preference,omitempty"`
	Enabled         bool   `json:"enabled"`
}

// Issue is the v0.1 issue wire shape.
type Issue struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Persona     string    `json:"persona,omitempty"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	// BatchID is set when this issue is part of a batch (POST /v1/batches).
	// Empty string for unbatched issues; the json:"omitempty" drops the
	// field on the wire so single-issue clients see no change.
	BatchID   string    `json:"batch_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Proposal is the agent-drafted change that an operator reviews on an issue.
// The shape is intentionally minimal: the body of the change in
// proposal_content (free-form, type-dependent), proposal_type to dispatch the
// right ability on approve, and target as a JSON blob with whatever the
// ability needs (e.g. {"product_id": 42}).
type Proposal struct {
	Type    string         `json:"type"`
	Content string         `json:"content"`
	Target  map[string]any `json:"target,omitempty"`
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.DB.QueryContext(r.Context(),
		`SELECT persona, name, COALESCE(model_preference, ''), enabled FROM agents ORDER BY persona`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	agents := []Persona{}
	for rows.Next() {
		var p Persona
		var enabled int
		if err := rows.Scan(&p.Persona, &p.Name, &p.ModelPreference, &enabled); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		p.Enabled = enabled != 0
		agents = append(agents, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	persona := r.URL.Query().Get("persona")
	batchID := r.URL.Query().Get("batch_id")

	q := `SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, COALESCE(batch_id, ''), created_at, updated_at FROM issues`
	args := []any{}
	where := []string{}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if persona != "" {
		where = append(where, "persona = ?")
		args = append(args, persona)
	}
	if batchID != "" {
		where = append(where, "batch_id = ?")
		args = append(args, batchID)
	}
	if len(where) > 0 {
		q += " WHERE " + joinAnd(where)
	}
	q += " ORDER BY created_at DESC LIMIT 500"

	rows, err := s.store.DB.QueryContext(ctx, q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	issues := []Issue{}
	for rows.Next() {
		var i Issue
		var createdAt, updatedAt string
		if err := rows.Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &i.BatchID, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		issues = append(issues, i)
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
}

type createIssueReq struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Persona     string    `json:"persona"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	BatchID     string    `json:"batch_id,omitempty"`
	Proposal    *Proposal `json:"proposal,omitempty"`
}

func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var req createIssueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}
	if req.Status == "" {
		req.Status = "backlog"
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}

	// If a batch_id was supplied, verify the parent batch exists. The FK
	// would catch the bad insert anyway, but the error would surface as a
	// generic db_error — pre-checking gives a clean 404.
	if req.BatchID != "" {
		var ok int
		err := s.store.DB.QueryRowContext(r.Context(),
			`SELECT 1 FROM batches WHERE id = ?`, req.BatchID,
		).Scan(&ok)
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "batch_not_found", "no batch with that id")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}

	var (
		proposalType    any
		proposalContent any
		proposalTarget  any
	)
	if req.Proposal != nil {
		if req.Proposal.Type == "" {
			writeError(w, http.StatusBadRequest, "missing_proposal_type", "proposal.type is required when proposal is set")
			return
		}
		proposalType = req.Proposal.Type
		proposalContent = req.Proposal.Content
		if len(req.Proposal.Target) > 0 {
			b, err := json.Marshal(req.Proposal.Target)
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_proposal_target", err.Error())
				return
			}
			proposalTarget = string(b)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := s.store.DB.ExecContext(r.Context(),
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target, batch_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Title, req.Description, nullIfEmpty(req.Persona), req.Status, req.Priority, now, now,
		proposalType, proposalContent, proposalTarget, nullIfEmpty(req.BatchID),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	ts, _ := time.Parse(time.RFC3339, now)
	writeJSON(w, http.StatusCreated, map[string]any{
		"issue": Issue{
			ID: id, Title: req.Title, Description: req.Description, Persona: req.Persona,
			Status: req.Status, Priority: req.Priority, BatchID: req.BatchID,
			CreatedAt: ts, UpdatedAt: ts,
		},
		"proposal": req.Proposal,
	})
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var i Issue
	var createdAt, updatedAt string
	var proposalType, proposalContent, proposalTarget sql.NullString
	err := s.store.DB.QueryRowContext(r.Context(),
		`SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, COALESCE(batch_id, ''), created_at, updated_at, proposal_type, proposal_content, proposal_target FROM issues WHERE id = ?`, id,
	).Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &i.BatchID, &createdAt, &updatedAt, &proposalType, &proposalContent, &proposalTarget)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no issue with that id")
		return
	}
	i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	var proposal *Proposal
	if proposalType.Valid && proposalType.String != "" {
		proposal = &Proposal{
			Type:    proposalType.String,
			Content: proposalContent.String,
		}
		if proposalTarget.Valid && proposalTarget.String != "" {
			_ = json.Unmarshal([]byte(proposalTarget.String), &proposal.Target)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"issue":    i,
		"runs":     []any{},
		"proposal": proposal,
	})
}

// approveDispatch maps a proposal_type to (a) the MCP ability that applies it
// and (b) a function that builds the ability's parameters from the proposal
// content + target. Adding new proposal types is purely additive — register a
// new dispatcher here, and the approve handler picks it up.
type approveDispatch struct {
	ability    string
	buildParams func(content string, target map[string]any) (map[string]any, error)
}

var approveDispatchByType = map[string]approveDispatch{
	"product_description_rewrite": {
		ability: "wooagent-products/update",
		buildParams: func(content string, target map[string]any) (map[string]any, error) {
			pid, err := requireIntFromTarget(target, "product_id")
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"id":          pid,
				"description": content,
			}, nil
		},
	},
	// Pricing persona. proposal.content is the operator-facing rationale
	// (sources cited, observed range, reasoning); the numeric payload lives
	// in proposal.target.regular_price (decimal string — Woo's update path
	// wants "19.99" not 19.99). Same MCP ability as the prose rewrite; the
	// fields-shipped subset is the only difference.
	"product_price_change": {
		ability: "wooagent-products/update",
		buildParams: func(_ string, target map[string]any) (map[string]any, error) {
			pid, err := requireIntFromTarget(target, "product_id")
			if err != nil {
				return nil, err
			}
			price, err := requireDecimalStringFromTarget(target, "regular_price")
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"id":            pid,
				"regular_price": price,
			}, nil
		},
	},
	// Sales Support persona. proposal.content is the message body (plain
	// text, ready for WP to email to the customer). target carries the
	// order id and a note_type discriminator — "customer" sets
	// is_customer_note=true so WP emails the note; "internal" leaves it
	// off so it shows only in wp-admin.
	"customer_reply_draft": {
		ability: "wooagent-orders/add-note",
		buildParams: func(content string, target map[string]any) (map[string]any, error) {
			oid, err := requireIntFromTarget(target, "order_id")
			if err != nil {
				return nil, err
			}
			note := strings.TrimSpace(content)
			if note == "" {
				return nil, fmt.Errorf("proposal content (note body) is empty")
			}
			isCustomer := true // default: customer-facing
			if v, ok := target["note_type"]; ok {
				if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), "internal") {
					isCustomer = false
				}
			}
			return map[string]any{
				"id":               oid,
				"note":             note,
				"is_customer_note": isCustomer,
			}, nil
		},
	},
}

// approveIssueReq is the optional body for POST /v1/issues/:id/approve. When
// a proposal carries multiple variants in target.variants[], the operator can
// pick one with variant_id and that variant's body becomes the description
// shipped to MCP. Empty body keeps the legacy single-proposal path: ship
// proposal_content as-is.
type approveIssueReq struct {
	VariantID string `json:"variant_id,omitempty"`
}

// approveResult is the success payload from approveOne. The single-issue
// HTTP handler maps it to the same JSON shape it has always returned; the
// batch approve-all loop appends its fields under a per-child results entry.
type approveResult struct {
	IssueID   string
	Status    string
	Ability   string
	AuditID   int64
	UpdatedAt string
}

// approveError is the typed failure shape returned by approveOne. The
// single-issue handler maps HTTPStatus + Code + Message to writeError, or
// uses PEPReason via writePEPDenial when set. The batch handler ignores
// HTTPStatus and embeds {code, message} per child in its 200 response.
type approveError struct {
	HTTPStatus int            // for the single-issue HTTP path
	Code       string         // stable error code; reused in batch per-child results
	Message    string         // human-readable
	PEPReason  pep.ReasonCode // non-empty iff this came from a PEP denial
}

// approveOne loads the issue + proposal, dispatches via PEP+MCP, and flips
// the status to done on success. No HTTP coupling — both the single-issue
// approve handler and the batch approve-all loop call this.
//
// Race-hardened: claims the issue with `UPDATE ... WHERE status='in_review'`
// before invoking PEP, gates on RowsAffected==1. Concurrent approvers that
// lose the race come back with code=wrong_status. On any failure after the
// claim flip we roll the status back to in_review so the operator can retry.
//
// The persona for PEP comes from issues.persona; we default to marketing when
// the issue has no persona set (V1 only has the marketing agent active).
func (s *Server) approveOne(ctx context.Context, issueID, variantID string) (approveResult, *approveError) {
	if s.pep == nil {
		return approveResult{}, &approveError{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       "mcp_not_configured",
			Message:    "daemon started without MCP credentials — set WOOAGENT_MCP_URL/USER/APP_PASSWORD and restart",
		}
	}

	// Single SELECT pulling status, persona, proposal, and batch_id at once.
	// Replaces the two-query pattern (load + loadIssuePersona) we used before
	// the batch-loop refactor.
	var status, proposalType, proposalContent string
	var personaSlug, proposalTarget, batchID sql.NullString
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status, persona, COALESCE(proposal_type, ''), COALESCE(proposal_content, ''), proposal_target, batch_id FROM issues WHERE id = ?`, issueID,
	).Scan(&status, &personaSlug, &proposalType, &proposalContent, &proposalTarget, &batchID)
	if err == sql.ErrNoRows {
		return approveResult{}, &approveError{HTTPStatus: http.StatusNotFound, Code: "not_found", Message: "no issue with that id"}
	}
	if err != nil {
		return approveResult{}, &approveError{HTTPStatus: http.StatusInternalServerError, Code: "db_error", Message: err.Error()}
	}
	if status != "in_review" {
		return approveResult{}, &approveError{HTTPStatus: http.StatusConflict, Code: "wrong_status", Message: "approve requires status=in_review, found " + status}
	}
	if proposalType == "" {
		return approveResult{}, &approveError{HTTPStatus: http.StatusUnprocessableEntity, Code: "no_proposal", Message: "issue has no proposal to approve"}
	}

	dispatch, ok := approveDispatchByType[proposalType]
	if !ok {
		return approveResult{}, &approveError{HTTPStatus: http.StatusUnprocessableEntity, Code: "unknown_proposal_type", Message: "no approve handler for proposal_type=" + proposalType}
	}

	target := map[string]any{}
	if proposalTarget.Valid && proposalTarget.String != "" {
		if err := json.Unmarshal([]byte(proposalTarget.String), &target); err != nil {
			return approveResult{}, &approveError{HTTPStatus: http.StatusInternalServerError, Code: "bad_target", Message: err.Error()}
		}
	}

	contentToShip := proposalContent
	if variantID != "" {
		body, err := resolveVariantBody(target, variantID)
		if err != nil {
			return approveResult{}, &approveError{HTTPStatus: http.StatusUnprocessableEntity, Code: "bad_variant_id", Message: err.Error()}
		}
		contentToShip = body
	}

	params, err := dispatch.buildParams(contentToShip, target)
	if err != nil {
		return approveResult{}, &approveError{HTTPStatus: http.StatusUnprocessableEntity, Code: "bad_proposal_target", Message: err.Error()}
	}

	// Race-hardening: claim the issue by flipping it to in_progress before
	// invoking PEP. RowsAffected==1 means we won; ==0 means another approver
	// got there first. The kanban briefly shows the card under Drafting
	// during this window — that's a feature, not a bug (it visualises that
	// work is in flight).
	now := time.Now().UTC().Format(time.RFC3339)
	claimRes, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET status = 'in_progress', updated_at = ? WHERE id = ? AND status = 'in_review'`, now, issueID,
	)
	if err != nil {
		return approveResult{}, &approveError{HTTPStatus: http.StatusInternalServerError, Code: "db_error", Message: err.Error()}
	}
	affected, _ := claimRes.RowsAffected()
	if affected != 1 {
		return approveResult{}, &approveError{HTTPStatus: http.StatusConflict, Code: "wrong_status", Message: "approve race lost — another approval already started"}
	}
	rollbackClaim := func() {
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE issues SET status = 'in_review', updated_at = ? WHERE id = ? AND status = 'in_progress'`,
			now, issueID,
		)
	}

	persona := manifest.PersonaMarketing
	if personaSlug.Valid && strings.TrimSpace(personaSlug.String) != "" {
		persona = manifest.Persona(personaSlug.String)
	}
	batchIDStr := ""
	if batchID.Valid {
		batchIDStr = batchID.String
	}

	// Route through the Policy Enforcement Point. The PEP runs the trust
	// state + persona scope checks (V1), writes a chain-of-identity audit
	// row, and dispatches to MCP. There is no direct s.mcp call site here
	// or anywhere else in the daemon — that's the §8.4.2 invariant.
	decision, mcpRes, invokeErr := s.pep.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: dispatch.ability,
		Args:    params,
		Intent:  pep.IntentApply,
		IssueID: issueID,
		BatchID: batchIDStr,
	})
	if !decision.Allowed {
		rollbackClaim()
		if invokeErr != nil {
			if errors.Is(invokeErr, pep.ErrMCPNotConfigured) {
				return approveResult{}, &approveError{HTTPStatus: http.StatusServiceUnavailable, Code: "mcp_not_configured", Message: "daemon started without MCP credentials — set WOOAGENT_MCP_URL/USER/APP_PASSWORD and restart"}
			}
			return approveResult{}, &approveError{HTTPStatus: http.StatusBadGateway, Code: "mcp_call_failed", Message: invokeErr.Error()}
		}
		return approveResult{}, &approveError{Code: string(decision.Reason), Message: pepDenialMessage(decision.Reason), PEPReason: decision.Reason}
	}

	if len(mcpRes.Content) == 0 {
		rollbackClaim()
		return approveResult{}, &approveError{HTTPStatus: http.StatusBadGateway, Code: "mcp_empty", Message: "MCP returned no content"}
	}
	var envelope struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(mcpRes.Content[0].Text), &envelope); err != nil {
		rollbackClaim()
		return approveResult{}, &approveError{HTTPStatus: http.StatusBadGateway, Code: "mcp_decode", Message: err.Error()}
	}
	if !envelope.Success {
		rollbackClaim()
		return approveResult{}, &approveError{HTTPStatus: http.StatusBadGateway, Code: "ability_failed", Message: envelope.Error}
	}

	if _, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET status = 'done', updated_at = ? WHERE id = ?`, now, issueID,
	); err != nil {
		return approveResult{}, &approveError{HTTPStatus: http.StatusInternalServerError, Code: "db_error", Message: err.Error()}
	}

	return approveResult{
		IssueID:   issueID,
		Status:    "done",
		Ability:   dispatch.ability,
		AuditID:   decision.AuditID,
		UpdatedAt: now,
	}, nil
}

// handleApproveIssue is now a thin HTTP wrapper around approveOne; the real
// work (PEP, MCP, status update) lives in the helper so the batch approve-all
// loop can share it.
func (s *Server) handleApproveIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req approveIssueReq
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
	}

	res, perr := s.approveOne(r.Context(), id, req.VariantID)
	if perr != nil {
		if perr.PEPReason != "" {
			writePEPDenial(w, perr.PEPReason)
			return
		}
		writeError(w, perr.HTTPStatus, perr.Code, perr.Message)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         res.IssueID,
		"status":     res.Status,
		"ability":    res.Ability,
		"audit_id":   res.AuditID,
		"updated_at": res.UpdatedAt,
	})
}

// writePEPDenial maps a typed PEP reason code to an HTTP status. Status
// choices follow the spirit of the codes: forbidden for trust/persona,
// unprocessable for schema/policy, too-many-requests for budget.
func writePEPDenial(w http.ResponseWriter, reason pep.ReasonCode) {
	switch reason {
	case pep.ReasonAbilityUnapproved, pep.ReasonPersonaForbidden, pep.ReasonScopeInsufficient:
		writeError(w, http.StatusForbidden, string(reason), pepDenialMessage(reason))
	case pep.ReasonInvalidArguments, pep.ReasonPolicyViolation:
		writeError(w, http.StatusUnprocessableEntity, string(reason), pepDenialMessage(reason))
	case pep.ReasonBudgetExceeded:
		writeError(w, http.StatusTooManyRequests, string(reason), pepDenialMessage(reason))
	default:
		writeError(w, http.StatusForbidden, "permission_denied", "PEP denied the call")
	}
}

func pepDenialMessage(reason pep.ReasonCode) string {
	switch reason {
	case pep.ReasonAbilityUnapproved:
		return "ability is not in the trusted manifest"
	case pep.ReasonPersonaForbidden:
		return "this persona is not permitted to invoke this ability"
	case pep.ReasonScopeInsufficient:
		return "intended action exceeds the ability's authorized scope"
	case pep.ReasonInvalidArguments:
		return "arguments did not validate against the ability's input schema"
	case pep.ReasonPolicyViolation:
		return "arguments tripped an operator-configured policy"
	case pep.ReasonBudgetExceeded:
		return "persona is over its daily budget"
	default:
		return "PEP denied the call"
	}
}

func (s *Server) handleRejectIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var status string
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status FROM issues WHERE id = ?`, id,
	).Scan(&status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "no issue with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if status != "in_review" {
		writeError(w, http.StatusConflict, "wrong_status",
			"reject requires status=in_review, found "+status)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET status = 'rejected', updated_at = ? WHERE id = ?`, now, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"status":     "rejected",
		"updated_at": now,
	})
}

// resolveVariantBody finds a variant by id inside target.variants[] and
// returns its body text. The shape mirrors the prototype's Variant type:
// each entry is a map with at least {id: string, body: string}.
func resolveVariantBody(target map[string]any, variantID string) (string, error) {
	raw, ok := target["variants"]
	if !ok {
		return "", fmt.Errorf("proposal target has no variants array")
	}
	list, ok := raw.([]any)
	if !ok {
		return "", fmt.Errorf("variants is not an array")
	}
	for _, v := range list {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		idStr, _ := m["id"].(string)
		if idStr != variantID {
			continue
		}
		body, ok := m["body"].(string)
		if !ok || body == "" {
			return "", fmt.Errorf("variant %s has no body", variantID)
		}
		return body, nil
	}
	return "", fmt.Errorf("variant_id %s not found in proposal target", variantID)
}

// requireIntFromTarget reads an integer value out of a JSON-decoded target
// map. SQLite stores the target as JSON text; on round-trip the numbers come
// back as float64, so we accept either shape.
func requireIntFromTarget(target map[string]any, key string) (int, error) {
	v, ok := target[key]
	if !ok {
		return 0, fmt.Errorf("missing %s in proposal target", key)
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("%s not numeric: %w", key, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("%s has unsupported type %T", key, v)
	}
}

// requireDecimalStringFromTarget reads a decimal price out of a JSON-decoded
// target. WooCommerce's update path wants a decimal string ("19.99"), but
// callers may have stored the value as a number — accept either and return a
// canonical 2dp string so the MCP write is deterministic.
func requireDecimalStringFromTarget(target map[string]any, key string) (string, error) {
	v, ok := target[key]
	if !ok {
		return "", fmt.Errorf("missing %s in proposal target", key)
	}
	switch n := v.(type) {
	case float64:
		if n <= 0 {
			return "", fmt.Errorf("%s must be > 0", key)
		}
		return strconv.FormatFloat(n, 'f', 2, 64), nil
	case int:
		if n <= 0 {
			return "", fmt.Errorf("%s must be > 0", key)
		}
		return strconv.FormatFloat(float64(n), 'f', 2, 64), nil
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return "", fmt.Errorf("%s is empty", key)
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return "", fmt.Errorf("%s not a decimal: %w", key, err)
		}
		if f <= 0 {
			return "", fmt.Errorf("%s must be > 0", key)
		}
		return strconv.FormatFloat(f, 'f', 2, 64), nil
	default:
		return "", fmt.Errorf("%s has unsupported type %T", key, v)
	}
}

func (s *Server) handleListAbilities(w http.ResponseWriter, r *http.Request) {
	// Populated once the MCP client lands Mon Apr 27.
	writeJSON(w, http.StatusOK, map[string]any{"abilities": []any{}})
}

// ---------- tiny helpers ----------

func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
