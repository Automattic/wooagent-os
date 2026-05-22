package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
)

// ModelProvider is the v0.1 model-provider wire shape. Matches
// ui/src/api/client.ts; secret material is never on the wire — the
// keychain holds it under secret_ref.
type ModelProvider struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	DefaultModel   string `json:"default_model"`
	IsDefault      bool   `json:"is_default,omitempty"`
	LastTestedAt   string `json:"last_tested_at,omitempty"`
	LastTestStatus string `json:"last_test_status,omitempty"`
}

const modelProviderSelectCols = `id, kind, name, endpoint, default_model, is_default, last_tested_at, last_test_status`

func scanModelProvider(scanner interface {
	Scan(...any) error
}) (ModelProvider, error) {
	var mp ModelProvider
	var endpoint, lastTestedAt sql.NullString
	var isDefault int
	if err := scanner.Scan(
		&mp.ID, &mp.Kind, &mp.Name, &endpoint, &mp.DefaultModel,
		&isDefault, &lastTestedAt, &mp.LastTestStatus,
	); err != nil {
		return ModelProvider{}, err
	}
	mp.Endpoint = endpoint.String
	mp.LastTestedAt = lastTestedAt.String
	mp.IsDefault = isDefault != 0
	return mp, nil
}

// validKinds gates create/test inputs. openai_compatible is intentionally
// out of scope for v0.1 onboarding (the design omits "Custom OpenAI-
// compatible" from the picker); add it here when Settings → Add provider
// grows that picker post-v0.1.
var validKinds = map[string]bool{
	"anthropic": true,
	"openai":    true,
	"ollama":    true,
}

// kindRequiresAPIKey reports whether the kind needs an API key (anthropic,
// openai do; ollama doesn't). Used by both /test and /create validation.
func kindRequiresAPIKey(kind string) bool {
	return kind == "anthropic" || kind == "openai"
}

// kindRequiresEndpoint reports whether the kind needs an operator-supplied
// endpoint URL. Anthropic / OpenAI use SDK defaults; Ollama doesn't.
func kindRequiresEndpoint(kind string) bool {
	return kind == "ollama"
}

// kindDisplayName is the human-readable "kind" label used to derive the
// default name on create. Editable post-create (PATCH lands post-v0.1).
func kindDisplayName(kind string) string {
	switch kind {
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "ollama":
		return "Ollama"
	}
	return kind
}

// handleTestModelProvider validates a (kind, api_key, endpoint, model) tuple
// against the upstream without persisting anything. Step 4 of onboarding
// disables Save until this returns ok=true. Always 200 — the endpoint
// itself completed; the failure is information about the configured
// upstream, encoded in the body.
func (s *Server) handleTestModelProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind         string `json:"kind"`
		APIKey       string `json:"api_key"`
		Endpoint     string `json:"endpoint"`
		DefaultModel string `json:"default_model"`
	}
	if !decodeJSONBody(w, r, &req, "invalid_json") {
		return
	}
	if !validKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "invalid_kind", "kind must be anthropic | openai | ollama")
		return
	}
	if kindRequiresAPIKey(req.Kind) && req.APIKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": "api_key is required for " + req.Kind})
		return
	}
	if kindRequiresEndpoint(req.Kind) && req.Endpoint == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": "endpoint is required for " + req.Kind})
		return
	}

	out := s.modelTester.Test(r.Context(), ModelTestArgs{
		Kind:         req.Kind,
		APIKey:       req.APIKey,
		Endpoint:     req.Endpoint,
		DefaultModel: req.DefaultModel,
	})

	body := map[string]any{"ok": out.OK}
	if out.Message != "" {
		body["message"] = out.Message
	}
	if out.Models != nil {
		body["models"] = out.Models
	}
	writeJSON(w, http.StatusOK, body)
}

// handleCreateModelProvider persists a new provider row + its API key in
// the keychain. The first row created is_default=1; subsequent rows are
// 0 (the partial unique index in migration 007 enforces this).
func (s *Server) handleCreateModelProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind         string `json:"kind"`
		APIKey       string `json:"api_key"`
		Endpoint     string `json:"endpoint"`
		DefaultModel string `json:"default_model"`
	}
	if !decodeJSONBody(w, r, &req, "invalid_json") {
		return
	}
	if !validKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "invalid_kind", "kind must be anthropic | openai | ollama")
		return
	}
	req.DefaultModel = strings.TrimSpace(req.DefaultModel)
	if req.DefaultModel == "" {
		writeError(w, http.StatusBadRequest, "missing_default_model", "default_model is required")
		return
	}
	if kindRequiresAPIKey(req.Kind) && req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "missing_api_key", "api_key is required for "+req.Kind)
		return
	}
	if kindRequiresEndpoint(req.Kind) && req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "missing_endpoint", "endpoint is required for "+req.Kind)
		return
	}

	ctx := r.Context()
	id := "mp_" + uuid.NewString()

	// First row in the table becomes the fleet default. Anything after
	// that defaults to is_default=0; the operator can promote later via a
	// future PATCH endpoint.
	var existing int
	if err := s.store.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM model_providers WHERE is_default = 1`,
	).Scan(&existing); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	isDefault := 0
	if existing == 0 {
		isDefault = 1
	}

	var secretRef sql.NullString
	if req.APIKey != "" {
		ref := "wooagent.model_providers." + id
		if err := s.secrets.Set(ctx, ref, req.APIKey); err != nil {
			writeError(w, http.StatusInternalServerError, "keychain_error", err.Error())
			return
		}
		secretRef = sql.NullString{String: ref, Valid: true}
	}

	name := kindDisplayName(req.Kind) + " · " + req.DefaultModel
	endpoint := sql.NullString{}
	if req.Endpoint != "" {
		endpoint = sql.NullString{String: req.Endpoint, Valid: true}
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.store.DB.ExecContext(ctx,
		`INSERT INTO model_providers(id, kind, name, endpoint, default_model, secret_ref, is_default, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Kind, name, endpoint, req.DefaultModel, secretRef, isDefault, now, now,
	); err != nil {
		// If the insert fails after the keychain write, we'd leak a secret.
		// Best-effort cleanup so the next attempt isn't blocked by an orphan.
		if secretRef.Valid {
			_ = s.secrets.Delete(ctx, secretRef.String)
		}
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	row := s.store.DB.QueryRowContext(ctx,
		`SELECT `+modelProviderSelectCols+` FROM model_providers WHERE id = ?`, id)
	mp, err := scanModelProvider(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, mp)
}

// handleListModelProviders returns every row, default-marked first so the
// UI can surface the fleet default at the top of the Settings → Models
// list without re-sorting client-side.
func (s *Server) handleListModelProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.DB.QueryContext(r.Context(),
		`SELECT `+modelProviderSelectCols+` FROM model_providers
		 ORDER BY is_default DESC, created_at ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	providers := []ModelProvider{}
	for rows.Next() {
		mp, err := scanModelProvider(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		providers = append(providers, mp)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

// handleDeleteModelProvider removes the row + its keychain entry.
//
// If the deleted row was is_default=1, we do NOT auto-promote another row
// — the UI surfaces "no default configured" and the operator picks
// explicitly via PATCH (post-v0.1). Implicit promotion would silently
// reroute persona traffic to a provider the operator may not have meant
// to make default.
func (s *Server) handleDeleteModelProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var secretRef sql.NullString
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT secret_ref FROM model_providers WHERE id = ?`, id,
	).Scan(&secretRef)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "provider_not_found", "no provider with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if secretRef.Valid && secretRef.String != "" {
		if err := s.secrets.Delete(ctx, secretRef.String); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "keychain_error", err.Error())
			return
		}
	}

	if _, err := s.store.DB.ExecContext(ctx, `DELETE FROM model_providers WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
