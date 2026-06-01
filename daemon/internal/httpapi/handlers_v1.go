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
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas/lessons"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// addableDefaultCadenceSeconds maps a persona slug to the cadence used
// when PATCH /v1/agents/{slug} first inserts the row (no operator-supplied
// cadence_seconds in the patch body). Slugs not in the map fall back to
// the agents.cadence_seconds column default (21600 / 6h). Tune per persona
// based on the work shape: digest-style personas (Reporting) want a longer
// interval than continuous-task personas (Marketing, Pricing).
var addableDefaultCadenceSeconds = map[string]int{
	"reporting": 86400, // 24h — Reporting is digest-style
}

// Persona is the v0.1 agent-persona wire shape.
//
// Implemented and Addable are sourced from the in-process personas registry,
// not from the agents table: they describe what the daemon *can* drive, not
// what's currently configured. The Agents screen uses them to decide
// whether to render full controls (toggle, model picker, Run now) or the
// "Coming soon" treatment.
type Persona struct {
	Persona         string  `json:"persona"`
	Name            string  `json:"name"`
	ModelPreference string  `json:"model_preference,omitempty"`
	Enabled         bool    `json:"enabled"`
	CadenceSeconds  int     `json:"cadence_seconds"`
	MaxAttempts     int     `json:"max_attempts"`
	LastRunAt       *string `json:"last_run_at"`
	NextRunAt       *string `json:"next_run_at"`
	// Implemented reports whether a Go-side persona is registered for this
	// slug — i.e. the daemon has a Draft() implementation. Unimplemented
	// slugs (Inventory, Accounting, Chief of Staff today) render as
	// "Coming soon" in the UI.
	Implemented bool `json:"implemented"`
	// Addable reports whether this persona ships dormant. Addable+disabled
	// rows surface via the Add Agent modal rather than getting full roster
	// controls until the operator opts in. Reporting is the only addable
	// persona today.
	Addable bool `json:"addable"`
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
	// Dismiss + archive metadata. Populated only when the operator dismissed
	// the issue via POST /v1/issues/:id/dismiss. The UI surfaces these on
	// the Archive screen. DSGWOO-1235.
	DismissReason  string     `json:"dismiss_reason,omitempty"`
	DismissComment string     `json:"dismiss_comment,omitempty"`
	DismissedAt    *time.Time `json:"dismissed_at,omitempty"`
	// UndoneAt is set by POST /v1/issues/:id/undo. When non-nil the
	// issue's approve has been reversed; status remains 'done'. The UI's
	// DoneBar uses this to render the "undone" affordance instead of Undo.
	UndoneAt *time.Time `json:"undone_at,omitempty"`
	// Target is the proposal's per-target payload (product_id, image_url,
	// etc.). Surfaced on list responses so queue card UIs can read fields
	// without fetching the full IssueDetail. Omitted when not set.
	Target json.RawMessage `json:"target,omitempty"`
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
		`SELECT persona, name, COALESCE(model_preference, ''), enabled,
		        cadence_seconds, max_attempts, last_run_at
		 FROM agents ORDER BY persona`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	agents := []Persona{}
	seen := map[string]bool{}
	for rows.Next() {
		var p Persona
		var enabled int
		var lastRunAt sql.NullString
		if err := rows.Scan(&p.Persona, &p.Name, &p.ModelPreference, &enabled,
			&p.CadenceSeconds, &p.MaxAttempts, &lastRunAt); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		p.Enabled = enabled != 0
		if lastRunAt.Valid && lastRunAt.String != "" {
			s := lastRunAt.String
			p.LastRunAt = &s
			if p.CadenceSeconds > 0 {
				if t, err := time.Parse(time.RFC3339, lastRunAt.String); err == nil {
					next := t.Add(time.Duration(p.CadenceSeconds) * time.Second).UTC().Format(time.RFC3339)
					p.NextRunAt = &next
				}
			}
		}
		annotateRegistryFlags(&p)
		seen[p.Persona] = true
		agents = append(agents, p)
	}

	// Merge in personas registered at runtime that aren't yet in the
	// agents table. These appear with Enabled=false so the UI can surface
	// them as addable (via the Add Agent modal -> PATCH /v1/agents/{slug}).
	// A persona absent from the registry but present in the DB (e.g. a
	// slug from an earlier daemon version) stays in the response as-is —
	// operators can still see / disable historical rows.
	for _, p := range personas.All() {
		slug := p.Slug()
		if seen[slug] {
			continue
		}
		agents = append(agents, Persona{
			Persona:     slug,
			Name:        p.DisplayName(),
			Enabled:     false,
			Implemented: true,
			Addable:     p.Addable(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// annotateRegistryFlags fills Implemented and Addable on p from the
// in-process personas registry. Slugs without a registry entry stay
// Implemented=false / Addable=false — that's the "Coming soon" treatment
// for personas the daemon can't currently drive (Inventory, Accounting,
// Chief of Staff today) and for historical DB rows whose persona has
// been removed from this build.
func annotateRegistryFlags(p *Persona) {
	if reg, ok := personas.Lookup(p.Persona); ok {
		p.Implemented = true
		p.Addable = reg.Addable()
	}
}

// patchAgentRequest is the v1 PATCH /v1/agents/{slug} body. All fields
// are optional pointers; only fields set in the request are written. This
// supports the three operator gestures (flip enabled, change model, change
// cadence) from a single endpoint, and lets the UI send partial updates
// without round-tripping the whole Persona shape.
type patchAgentRequest struct {
	Enabled         *bool   `json:"enabled,omitempty"`
	Name            *string `json:"name,omitempty"`
	ModelPreference *string `json:"model_preference,omitempty"`
	CadenceSeconds  *int    `json:"cadence_seconds,omitempty"`
	// ApplyHoursStart and ApplyHoursEnd are HH:MM 24-hour strings that gate
	// when the agent may run. Both must be set together or both unset
	// (XOR is rejected). See DSGWOO-1282.
	ApplyHoursStart *string `json:"apply_hours_start,omitempty"`
	ApplyHoursEnd   *string `json:"apply_hours_end,omitempty"`
}

// handlePatchAgent applies a partial update to the agents row for the slug
// in the URL. The persona must be registered in the runtime registry —
// otherwise 404. If no row exists for the persona yet (e.g. an addable
// persona that the operator is opting in for the first time), we INSERT
// with sensible defaults sourced from the registry + addableDefaultCadenceSeconds,
// then UPDATE with whatever the patch supplied. Returns the canonical row
// post-update so the UI can refresh optimistically without a second round-trip.
func (s *Server) handlePatchAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	regPersona, ok := personas.Lookup(slug)
	if !ok {
		writeError(w, http.StatusNotFound, "persona_not_registered",
			fmt.Sprintf("no persona %q is registered in this daemon build", slug))
		return
	}

	var req patchAgentRequest
	if r.ContentLength != 0 {
		if !decodeJSONBody(w, r, &req, "bad_request") {
			return
		}
	}

	// Apply-hours validation: both must be HH:MM 24-hour or both unset.
	// XOR is rejected. policy_hours.go re-validates as defense-in-depth;
	// this surfaces the error to the operator at write time.
	hasStart := req.ApplyHoursStart != nil && *req.ApplyHoursStart != ""
	hasEnd := req.ApplyHoursEnd != nil && *req.ApplyHoursEnd != ""
	if hasStart != hasEnd {
		writeError(w, http.StatusBadRequest, "invalid_apply_hours",
			"both apply_hours_start and apply_hours_end must be set, or both unset")
		return
	}
	if hasStart && !pep.ValidHHMM(*req.ApplyHoursStart) {
		writeError(w, http.StatusBadRequest, "invalid_apply_hours_start",
			"apply_hours_start must be HH:MM 24-hour format")
		return
	}
	if hasEnd && !pep.ValidHHMM(*req.ApplyHoursEnd) {
		writeError(w, http.StatusBadRequest, "invalid_apply_hours_end",
			"apply_hours_end must be HH:MM 24-hour format")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Ensure a row exists for the persona. INSERT OR IGNORE keeps the
	// existing row untouched when present; on first-touch it seeds the
	// row with sensible defaults from the registry (display name) plus
	// the per-persona cadence override if any. enabled starts at 0; the
	// UPDATE below flips it if the patch requested.
	defaultCadence := addableDefaultCadenceSeconds[slug]
	if defaultCadence > 0 {
		_, err := s.store.DB.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO agents(persona, name, cadence_seconds, enabled, created_at, updated_at)
			 VALUES(?, ?, ?, 0, ?, ?)`,
			slug, regPersona.DisplayName(), defaultCadence, now, now,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	} else {
		_, err := s.store.DB.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO agents(persona, name, enabled, created_at, updated_at)
			 VALUES(?, ?, 0, ?, ?)`,
			slug, regPersona.DisplayName(), now, now,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}

	// Apply the patch. Each field is set independently so a request that
	// only touches one column doesn't clobber the others. updated_at
	// always advances on a PATCH, even if the body was empty — the empty
	// body case is treated as "touch this row" rather than rejected; that
	// keeps the contract simple for the UI's optimistic-refresh path.
	setParts := []string{"updated_at = ?"}
	args := []any{now}
	if req.Enabled != nil {
		setParts = append(setParts, "enabled = ?")
		enabledInt := 0
		if *req.Enabled {
			enabledInt = 1
		}
		args = append(args, enabledInt)
	}
	if req.Name != nil {
		setParts = append(setParts, "name = ?")
		args = append(args, *req.Name)
	}
	if req.ModelPreference != nil {
		setParts = append(setParts, "model_preference = ?")
		args = append(args, *req.ModelPreference)
	}
	if req.CadenceSeconds != nil {
		setParts = append(setParts, "cadence_seconds = ?")
		args = append(args, *req.CadenceSeconds)
	}
	if hasStart {
		setParts = append(setParts, "apply_hours_start = ?")
		args = append(args, *req.ApplyHoursStart)
		setParts = append(setParts, "apply_hours_end = ?")
		args = append(args, *req.ApplyHoursEnd)
	}
	args = append(args, slug)

	updateSQL := "UPDATE agents SET " + strings.Join(setParts, ", ") + " WHERE persona = ?"
	if _, err := s.store.DB.ExecContext(r.Context(), updateSQL, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Read back the canonical row so the response matches what GET
	// /v1/agents will return.
	var persona Persona
	var enabled int
	var lastRunAt sql.NullString
	err := s.store.DB.QueryRowContext(r.Context(),
		`SELECT persona, name, COALESCE(model_preference, ''), enabled,
		        cadence_seconds, max_attempts, last_run_at
		 FROM agents WHERE persona = ?`, slug,
	).Scan(&persona.Persona, &persona.Name, &persona.ModelPreference, &enabled,
		&persona.CadenceSeconds, &persona.MaxAttempts, &lastRunAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_readback", err.Error())
		return
	}
	persona.Enabled = enabled != 0
	if lastRunAt.Valid && lastRunAt.String != "" {
		v := lastRunAt.String
		persona.LastRunAt = &v
	}
	annotateRegistryFlags(&persona)
	writeJSON(w, http.StatusOK, persona)
}

// lessonsResponse is the GET /v1/agents/{slug}/lessons body. (DSGWOO-1354)
type lessonsResponse struct {
	Persona      string `json:"persona"`
	LessonsText  string `json:"lessons_text"`
	GeneratedAt  string `json:"generated_at"`
	SourceCount  int    `json:"source_count"`
	SourceOldest string `json:"source_oldest_dismissed_at"`
	SourceNewest string `json:"source_newest_dismissed_at"`
	Disabled     bool   `json:"disabled"`
}

// handleGetLessons returns the current persona_lessons row for {slug}, or 404
// if none has been digested yet. (DSGWOO-1354)
func (s *Server) handleGetLessons(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var resp lessonsResponse
	err := s.store.DB.QueryRowContext(r.Context(),
		`SELECT persona, lessons_text, generated_at, source_count,
		        source_oldest_dismissed_at, source_newest_dismissed_at
		 FROM persona_lessons WHERE persona = ?`, slug).
		Scan(&resp.Persona, &resp.LessonsText, &resp.GeneratedAt, &resp.SourceCount, &resp.SourceOldest, &resp.SourceNewest)
	if err == sql.ErrNoRows {
		http.Error(w, "no lessons for that persona", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "lessons lookup failed", http.StatusInternalServerError)
		return
	}
	resp.Disabled = lessons.DisabledFor(slug)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	persona := r.URL.Query().Get("persona")
	batchID := r.URL.Query().Get("batch_id")

	q := `SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, COALESCE(batch_id, ''), created_at, updated_at, COALESCE(dismiss_reason, ''), COALESCE(dismiss_comment, ''), dismissed_at, undone_at, COALESCE(proposal_target, 'null') FROM issues`
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
		var dismissedAt, undoneAt sql.NullString
		var targetRaw string
		if err := rows.Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &i.BatchID, &createdAt, &updatedAt, &i.DismissReason, &i.DismissComment, &dismissedAt, &undoneAt, &targetRaw); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if dismissedAt.Valid && dismissedAt.String != "" {
			if t, err := time.Parse(time.RFC3339, dismissedAt.String); err == nil {
				i.DismissedAt = &t
			}
		}
		if undoneAt.Valid && undoneAt.String != "" {
			if t, err := time.Parse(time.RFC3339, undoneAt.String); err == nil {
				i.UndoneAt = &t
			}
		}
		if targetRaw != "" && targetRaw != "null" {
			i.Target = json.RawMessage(targetRaw)
		}
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
	if !decodeJSONBody(w, r, &req, "bad_json") {
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
	var proposalType, proposalContent, proposalTarget, dismissedAt, undoneAt sql.NullString
	err := s.store.DB.QueryRowContext(r.Context(),
		`SELECT id, title, COALESCE(description, ''), COALESCE(persona, ''), status, priority, COALESCE(batch_id, ''), created_at, updated_at, proposal_type, proposal_content, proposal_target, COALESCE(dismiss_reason, ''), COALESCE(dismiss_comment, ''), dismissed_at, undone_at FROM issues WHERE id = ?`, id,
	).Scan(&i.ID, &i.Title, &i.Description, &i.Persona, &i.Status, &i.Priority, &i.BatchID, &createdAt, &updatedAt, &proposalType, &proposalContent, &proposalTarget, &i.DismissReason, &i.DismissComment, &dismissedAt, &undoneAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no issue with that id")
		return
	}
	i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if dismissedAt.Valid && dismissedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, dismissedAt.String); err == nil {
			i.DismissedAt = &t
		}
	}
	if undoneAt.Valid && undoneAt.String != "" {
		if t, err := time.Parse(time.RFC3339, undoneAt.String); err == nil {
			i.UndoneAt = &t
		}
	}

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
// content + target. buildParams returns the params payload AND the string
// snapshot of the value we wrote (decimal string for prices, full body for
// rewrites). approveOne persists the snapshot into issues.applied_value so
// the undo handler's staleness check has a comparable. Dispatchers that
// can't be undone (cold-draft, customer-reply) return "" as appliedValue.
//
// selectedVariant is the full variant map chosen by the operator (nil when
// no variant_id was supplied). Dispatchers that only need the content string
// (rewrite, price-change, customer-reply) ignore it; cold-draft reads
// body_short/body_long from it.
type approveDispatch struct {
	ability     string
	buildParams func(content string, selectedVariant map[string]any, target map[string]any) (params map[string]any, appliedValue string, err error)
}

var approveDispatchByType = map[string]approveDispatch{
	"product_description_rewrite": {
		ability: "wooagent-products/update",
		buildParams: func(content string, _ map[string]any, target map[string]any) (map[string]any, string, error) {
			pid, err := requireIntFromTarget(target, "product_id")
			if err != nil {
				return nil, "", err
			}
			return map[string]any{
				"id":          pid,
				"description": content,
			}, content, nil
		},
	},
	// Pricing persona. proposal.content is the operator-facing rationale
	// (sources cited, observed range, reasoning); the numeric payload lives
	// in proposal.target.regular_price (decimal string — Woo's update path
	// wants "19.99" not 19.99). target.target_field selects which Woo field
	// receives the new value ("regular_price" or "sale_price"); the
	// dispatcher reads target["regular_price"] as the decimal string to
	// write regardless of which field it targets (key name kept for
	// back-compat with in-flight proposals). Same MCP ability as the prose
	// rewrite; the fields-shipped subset is the only difference.
	"product_price_change": {
		ability: "wooagent-products/update",
		buildParams: func(_ string, _ map[string]any, target map[string]any) (map[string]any, string, error) {
			pid, err := requireIntFromTarget(target, "product_id")
			if err != nil {
				return nil, "", err
			}
			field := "regular_price"
			if v, present := target["target_field"]; present {
				s, ok := v.(string)
				if !ok {
					return nil, "", fmt.Errorf("target_field must be a string, got %T", v)
				}
				s = strings.TrimSpace(s)
				switch s {
				case "regular_price", "sale_price":
					field = s
				case "":
					// back-compat: empty falls back to regular_price
				default:
					return nil, "", fmt.Errorf("invalid target_field %q (expected regular_price|sale_price)", s)
				}
			}
			price, err := requireDecimalStringFromTarget(target, "regular_price")
			if err != nil {
				return nil, "", err
			}
			return map[string]any{"id": pid, field: price}, price, nil
		},
	},
	// Sales Support persona. proposal.content is the message body (plain
	// text, ready for WP to email to the customer). target carries the
	// order id and a note_type discriminator — "customer" sets
	// is_customer_note=true so WP emails the note; "internal" leaves it
	// off so it shows only in wp-admin. Not undoable in v1 — the
	// dispatcher returns "" as appliedValue so the undo handler will
	// reject any attempt with code=not_undoable.
	"customer_reply_draft": {
		ability: "wooagent-orders/add-note",
		buildParams: func(content string, _ map[string]any, target map[string]any) (map[string]any, string, error) {
			oid, err := requireIntFromTarget(target, "order_id")
			if err != nil {
				return nil, "", err
			}
			note := strings.TrimSpace(content)
			if note == "" {
				return nil, "", fmt.Errorf("proposal content (note body) is empty")
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
			}, "", nil
		},
	},
	// Marketing persona — cold-draft path. proposal.target.drafting lists
	// which description fields to fill ("short" and/or "long"); the selected
	// variant carries body_short and/or body_long. The payload writes only
	// the previously-empty fields; existing copy in non-drafted fields is
	// preserved because we only include keys named in drafting. Not undoable
	// in v1 (the natural reverse — writing an empty string back — is
	// rarely what an operator wants); dispatcher returns "" as appliedValue.
	"product_cold_draft": {
		ability: "wooagent-products/update",
		buildParams: func(_ string, selectedVariant map[string]any, target map[string]any) (map[string]any, string, error) {
			pid, err := requireIntFromTarget(target, "product_id")
			if err != nil {
				return nil, "", err
			}
			if selectedVariant == nil {
				return nil, "", fmt.Errorf("cold-draft approval requires a variant_id")
			}
			rawDrafting, ok := target["drafting"]
			if !ok {
				return nil, "", fmt.Errorf("missing drafting in proposal target")
			}
			drafting, ok := rawDrafting.([]any)
			if !ok {
				return nil, "", fmt.Errorf("drafting is not an array")
			}
			params := map[string]any{"id": pid}
			for _, f := range drafting {
				field, ok := f.(string)
				if !ok {
					return nil, "", fmt.Errorf("drafting entry %v is not a string", f)
				}
				switch field {
				case "short":
					short, ok := selectedVariant["body_short"].(string)
					if !ok || short == "" {
						return nil, "", fmt.Errorf("variant missing body_short (drafting requires it)")
					}
					params["short_description"] = short
				case "long":
					long, ok := selectedVariant["body_long"].(string)
					if !ok || long == "" {
						return nil, "", fmt.Errorf("variant missing body_long (drafting requires it)")
					}
					params["description"] = long
				default:
					return nil, "", fmt.Errorf("unknown drafting field %q (expected short|long)", field)
				}
			}
			return params, "", nil
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
	var selectedVariant map[string]any
	if variantID != "" {
		v, err := resolveSelectedVariant(target, variantID)
		if err != nil {
			return approveResult{}, &approveError{HTTPStatus: http.StatusUnprocessableEntity, Code: "bad_variant_id", Message: err.Error()}
		}
		selectedVariant = v
		// For legacy variants (have body), keep populating contentToShip so
		// existing dispatchers (rewrite, customer-reply) continue to receive
		// the variant's body string unchanged.
		if body, ok := v["body"].(string); ok && body != "" {
			contentToShip = body
		}
	}

	params, applied, err := dispatch.buildParams(contentToShip, selectedVariant, target)
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
	//
	// This Invoke mints a one-shot apply permission scoped to (IssueID,
	// Ability, Args). It does not persist beyond this call: a second
	// /approve POST returns 409 wrong_status because the in_review → done
	// status flip below blocks replay. Per-ability trust (abilities table)
	// is the separate, lasting layer; see §8.4.2 for the two-layer model.
	decision, mcpRes, invokeErr := s.pep.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: dispatch.ability,
		Args:    params,
		Intent:  pep.IntentApply,
		Source:  pep.SourceOperator,
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

	var appliedArg any = nil
	if applied != "" {
		appliedArg = applied
	}
	if _, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET status = 'done', applied_value = ?, updated_at = ? WHERE id = ?`,
		appliedArg, now, issueID,
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
		if !decodeJSONBody(w, r, &req, "bad_json") {
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

	// Attach the operator verdict to the issue's most-recent turn so the
	// GEPA pipeline can pair the proposal with the approval signal.
	// Best-effort; the approve already succeeded. DSGWOO-1236.
	_ = telemetry.RecordVerdict(r.Context(), s.store.DB, res.IssueID, telemetry.Verdict{
		Kind:      telemetry.VerdictApprove,
		DecidedAt: time.Now().UTC(),
	})

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
	case pep.ReasonSchemaCompileError:
		writeError(w, http.StatusInternalServerError, string(reason), pepDenialMessage(reason))
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
		return "persona is over its daily budget — counters reset at local midnight"
	case pep.ReasonSchemaCompileError:
		return "the local schema cache for this ability is invalid — try re-discovering abilities for this store"
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

	_ = telemetry.RecordVerdict(ctx, s.store.DB, id, telemetry.Verdict{
		Kind:      telemetry.VerdictReject,
		DecidedAt: time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"status":     "rejected",
		"updated_at": now,
	})
}

// dismissIssueReq is the body for POST /v1/issues/:id/dismiss. Reason is
// required (one of the DismissReason values defined in ui/src/api/client.ts);
// comment is optional free-text the operator added in the dialog textarea.
type dismissIssueReq struct {
	Reason  string `json:"reason"`
	Comment string `json:"comment,omitempty"`
}

// handleDismissIssue transitions an in_review issue to 'dismissed', captures
// the operator-supplied reason + optional comment, and stamps dismissed_at
// so the 30-day TTL countdown can run. UI surfaces these on the Archive
// screen. DSGWOO-1235.
func (s *Server) handleDismissIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var req dismissIssueReq
	if !decodeJSONBody(w, r, &req, "bad_body") {
		return
	}
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "missing_reason",
			"dismiss requires a reason field")
		return
	}

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
			"dismiss requires status=in_review, found "+status)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var commentArg any
	if req.Comment != "" {
		commentArg = req.Comment
	}
	if _, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues
		 SET status = 'dismissed',
		     dismiss_reason = ?,
		     dismiss_comment = ?,
		     dismissed_at = ?,
		     updated_at = ?
		 WHERE id = ?`,
		req.Reason, commentArg, now, now, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	_ = telemetry.RecordVerdict(ctx, s.store.DB, id, telemetry.Verdict{
		Kind:       telemetry.VerdictDismiss,
		ReasonTag:  req.Reason,
		ReasonText: req.Comment,
		DecidedAt:  time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"id":              id,
		"status":          "dismissed",
		"dismiss_reason":  req.Reason,
		"dismiss_comment": req.Comment,
		"dismissed_at":    now,
		"updated_at":      now,
	})
}

// resolveSelectedVariant finds the variant by id inside target.variants[]
// and returns its full map. Cold-draft dispatch needs the structured
// body_short/body_long fields, not just a single body string.
func resolveSelectedVariant(target map[string]any, variantID string) (map[string]any, error) {
	raw, ok := target["variants"]
	if !ok {
		return nil, fmt.Errorf("proposal target has no variants array")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("variants is not an array")
	}
	for _, v := range list {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if idStr, _ := m["id"].(string); idStr == variantID {
			return m, nil
		}
	}
	return nil, fmt.Errorf("variant_id %s not found in proposal target", variantID)
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
