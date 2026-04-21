package httpapi

import (
	"encoding/json"
	"net/http"
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
	Title       string `json:"title"`
	Description string `json:"description"`
	Persona     string `json:"persona"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
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

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := s.store.DB.ExecContext(r.Context(),
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Title, req.Description, nullIfEmpty(req.Persona), req.Status, req.Priority, now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	ts, _ := time.Parse(time.RFC3339, now)
	writeJSON(w, http.StatusCreated, Issue{
		ID: id, Title: req.Title, Description: req.Description, Persona: req.Persona,
		Status: req.Status, Priority: req.Priority, CreatedAt: ts, UpdatedAt: ts,
	})
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var i Issue
	var createdAt, updatedAt string
	err := s.store.DB.QueryRowContext(r.Context(),
		`SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, created_at, updated_at FROM issues WHERE id = ?`, id,
	).Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &createdAt, &updatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no issue with that id")
		return
	}
	i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"issue": i,
		"runs":  []any{},
		// Diff proposals land when propose-mode ships in Phase 2.
		"proposal": nil,
	})
}

func (s *Server) handleApproveIssue(w http.ResponseWriter, r *http.Request) {
	// Phase 2 will invoke the staged MCP ability and move the issue to "done".
	writeError(w, http.StatusNotImplemented, "not_implemented", "approve lands in Phase 2 (review/approval)")
}

func (s *Server) handleRejectIssue(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "reject lands in Phase 2 (review/approval)")
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
