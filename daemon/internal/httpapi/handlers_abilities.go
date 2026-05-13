package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Ability is the v0.1 wire shape for /v1/abilities. Mirrors the abilities
// table; the schema body is sent as raw JSON when present so the UI's
// "view schema" pane doesn't have to round-trip through a typed struct.
//
// trust_state values:
//   new             — operator has never approved this ability
//   trusted         — approved at the schema currently cached
//   schema_changed  — approved earlier, but the upstream schema has drifted
type Ability struct {
	ID          string          `json:"id"`
	StoreID     string          `json:"store_id"`
	StoreURL    string          `json:"store_url,omitempty"`
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Version     string          `json:"version,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	SchemaHash  string          `json:"schema_hash,omitempty"`
	TrustState  string          `json:"trust_state"`
	// EffectiveTrust is the UI-facing trust label, derived from
	// TrustState and manifest-pre-signing. Allowed values:
	// "built-in" | "trusted" | "needs_review" | "schema_changed".
	// Manifest-pre-signed abilities surface as "built-in" even when
	// TrustState == "new", because the PEP admits them regardless.
	EffectiveTrust string `json:"effective_trust"`
	TrustedAt   string          `json:"trusted_at,omitempty"`
	LastSeenAt  string          `json:"last_seen_at,omitempty"`
}

// computeEffectiveTrust derives the UI-facing trust label from the raw
// DB trust_state and whether the ability is pre-signed in the bundled
// manifest. The PEP already admits manifest-pre-signed calls regardless
// of trust_state (pep.checkTrustState); this surface mirrors that so the
// UI stops claiming "needs review" for abilities that already work.
func computeEffectiveTrust(trustState string, manifestSigned bool) string {
	switch {
	case manifestSigned && trustState == "schema_changed":
		return "schema_changed"
	case manifestSigned:
		return "built-in"
	case trustState == "trusted":
		return "trusted"
	case trustState == "schema_changed":
		return "schema_changed"
	default:
		return "needs_review"
	}
}

// handleListAbilities returns abilities for one or all paired stores.
//
// Optional query params (AND'd together):
//   store_id     — filter to one store
//   trust_state  — new | trusted | schema_changed
//   (The UI's Status filter uses effective_trust values — built-in,
//   trusted, needs_review, schema_changed — but filtering happens
//   client-side via DataViews; this query param is for non-UI consumers.)
func (s *Server) handleListAbilities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	where := []string{}
	args := []any{}
	if v := strings.TrimSpace(q.Get("store_id")); v != "" {
		where = append(where, "abilities.store_id = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("trust_state")); v != "" {
		switch v {
		case "new", "trusted", "schema_changed":
			where = append(where, "abilities.trust_state = ?")
			args = append(args, v)
		default:
			writeError(w, http.StatusBadRequest, "invalid_trust_state",
				"trust_state must be one of: new, trusted, schema_changed")
			return
		}
	}

	sqlText := `SELECT abilities.id, abilities.store_id, stores.url, abilities.name,
	                  COALESCE(abilities.title,''), COALESCE(abilities.description,''),
	                  COALESCE(abilities.version,''), COALESCE(abilities.schema_json,''),
	                  abilities.schema_hash, abilities.trust_state,
	                  COALESCE(abilities.trusted_at,''), abilities.last_seen_at
	             FROM abilities
	             JOIN stores ON stores.id = abilities.store_id`
	if len(where) > 0 {
		sqlText += " WHERE " + joinAnd(where)
	}
	sqlText += " ORDER BY stores.url, abilities.name"

	rows, err := s.store.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	out := []Ability{}
	for rows.Next() {
		var ab Ability
		var schemaJSON string
		if err := rows.Scan(
			&ab.ID, &ab.StoreID, &ab.StoreURL, &ab.Name,
			&ab.Title, &ab.Description, &ab.Version,
			&schemaJSON, &ab.SchemaHash, &ab.TrustState,
			&ab.TrustedAt, &ab.LastSeenAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		if schemaJSON != "" {
			ab.Schema = json.RawMessage(schemaJSON)
		}
		manifestSigned := false
		if m := s.pep.Manifest(); m != nil && m.Get(ab.Name) != nil {
			manifestSigned = true
		}
		ab.EffectiveTrust = computeEffectiveTrust(ab.TrustState, manifestSigned)
		out = append(out, ab)
	}
	writeJSON(w, http.StatusOK, map[string]any{"abilities": out})
}

// handleTrustAbility flips an ability's trust_state to 'trusted' and
// captures its current schema_hash as trusted_hash. Idempotent — already
// trusted rows are returned unchanged. Used by the operator-clicks-Approve
// path in the Abilities browser.
func (s *Server) handleTrustAbility(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var schemaHash, trustState string
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT schema_hash, trust_state FROM abilities WHERE id = ?`, id,
	).Scan(&schemaHash, &trustState)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ability_not_found", "no ability with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.store.DB.ExecContext(ctx,
		`UPDATE abilities SET trust_state='trusted', trusted_hash=?, trusted_at=?, updated_at=? WHERE id=?`,
		schemaHash, now, now, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	s.respondAbilityByID(w, r, id, http.StatusOK)
}

// respondAbilityByID is the shared single-row read path — used by the
// trust handler so its response body matches the list shape.
func (s *Server) respondAbilityByID(w http.ResponseWriter, r *http.Request, id string, status int) {
	row := s.store.DB.QueryRowContext(r.Context(),
		`SELECT abilities.id, abilities.store_id, stores.url, abilities.name,
		        COALESCE(abilities.title,''), COALESCE(abilities.description,''),
		        COALESCE(abilities.version,''), COALESCE(abilities.schema_json,''),
		        abilities.schema_hash, abilities.trust_state,
		        COALESCE(abilities.trusted_at,''), abilities.last_seen_at
		   FROM abilities
		   JOIN stores ON stores.id = abilities.store_id
		  WHERE abilities.id = ?`, id)
	var ab Ability
	var schemaJSON string
	if err := row.Scan(
		&ab.ID, &ab.StoreID, &ab.StoreURL, &ab.Name,
		&ab.Title, &ab.Description, &ab.Version,
		&schemaJSON, &ab.SchemaHash, &ab.TrustState,
		&ab.TrustedAt, &ab.LastSeenAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if schemaJSON != "" {
		ab.Schema = json.RawMessage(schemaJSON)
	}
	manifestSigned := false
	if m := s.pep.Manifest(); m != nil && m.Get(ab.Name) != nil {
		manifestSigned = true
	}
	ab.EffectiveTrust = computeEffectiveTrust(ab.TrustState, manifestSigned)
	writeJSON(w, status, ab)
}
