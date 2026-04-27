package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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

	q := `SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, created_at, updated_at FROM issues`
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
		if err := rows.Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &createdAt, &updatedAt); err != nil {
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
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Persona     string         `json:"persona"`
	Priority    string         `json:"priority"`
	Status      string         `json:"status"`
	Proposal    *Proposal      `json:"proposal,omitempty"`
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
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Title, req.Description, nullIfEmpty(req.Persona), req.Status, req.Priority, now, now,
		proposalType, proposalContent, proposalTarget,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	ts, _ := time.Parse(time.RFC3339, now)
	writeJSON(w, http.StatusCreated, map[string]any{
		"issue": Issue{
			ID: id, Title: req.Title, Description: req.Description, Persona: req.Persona,
			Status: req.Status, Priority: req.Priority, CreatedAt: ts, UpdatedAt: ts,
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
		`SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target FROM issues WHERE id = ?`, id,
	).Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &createdAt, &updatedAt, &proposalType, &proposalContent, &proposalTarget)
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
}

func (s *Server) handleApproveIssue(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_not_configured",
			"daemon started without MCP credentials — set WOOAGENT_MCP_URL/USER/APP_PASSWORD and restart")
		return
	}

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// Load the issue + proposal in one shot. Status check happens against the
	// just-loaded value so two concurrent approves can't both fire (the second
	// one will see status="done" and bail).
	var status, proposalType, proposalContent string
	var proposalTarget sql.NullString
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status, COALESCE(proposal_type, ''), COALESCE(proposal_content, ''), proposal_target FROM issues WHERE id = ?`, id,
	).Scan(&status, &proposalType, &proposalContent, &proposalTarget)
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
			"approve requires status=in_review, found "+status)
		return
	}
	if proposalType == "" {
		writeError(w, http.StatusUnprocessableEntity, "no_proposal",
			"issue has no proposal to approve")
		return
	}

	dispatch, ok := approveDispatchByType[proposalType]
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "unknown_proposal_type",
			"no approve handler for proposal_type="+proposalType)
		return
	}

	target := map[string]any{}
	if proposalTarget.Valid && proposalTarget.String != "" {
		if err := json.Unmarshal([]byte(proposalTarget.String), &target); err != nil {
			writeError(w, http.StatusInternalServerError, "bad_target", err.Error())
			return
		}
	}

	params, err := dispatch.buildParams(proposalContent, target)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", err.Error())
		return
	}

	// Ensure the MCP session is alive. Initialize is idempotent — if we already
	// have a session, the client returns the cached server info.
	if _, err := s.mcp.Initialize(ctx); err != nil {
		writeError(w, http.StatusBadGateway, "mcp_init_failed", err.Error())
		return
	}

	// Route through the WordPress MCP Adapter's three-meta-tool pattern, the
	// same shape spike-mcp + persona-marketing use.
	res, err := s.mcp.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": dispatch.ability,
		"parameters":   params,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "mcp_call_failed", err.Error())
		return
	}
	if len(res.Content) == 0 {
		writeError(w, http.StatusBadGateway, "mcp_empty", "MCP returned no content")
		return
	}
	var envelope struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &envelope); err != nil {
		writeError(w, http.StatusBadGateway, "mcp_decode", err.Error())
		return
	}
	if !envelope.Success {
		writeError(w, http.StatusBadGateway, "ability_failed", envelope.Error)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET status = 'done', updated_at = ? WHERE id = ?`, now, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"status":     "done",
		"ability":    dispatch.ability,
		"updated_at": now,
	})
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
