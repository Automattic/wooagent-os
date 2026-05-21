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

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// undoableProposalTypes enumerates the proposal types Undo can reverse.
// Adding a new type means: (1) the dispatcher must emit a non-empty
// appliedValue in approveOne, and (2) buildUndoParams below must know
// how to derive the reverse-write payload from the original proposal.
var undoableProposalTypes = map[string]struct{}{
	"product_price_change":          {},
	"product_price_change_variable": {},
	"product_description_rewrite":   {},
}

// undoParamsErr lets buildUndoParams return typed failures without
// importing approveError's HTTP semantics.
type undoParamsErr struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (s *Server) handleUndoIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.pep == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_not_configured",
			"daemon started without MCP credentials — set WOOAGENT_MCP_URL/USER/APP_PASSWORD and restart")
		return
	}

	ctx := r.Context()
	var status, proposalType, proposalContent string
	var personaSlug, proposalTarget, batchID, appliedValue, undoneAt sql.NullString
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status, persona, COALESCE(proposal_type, ''), COALESCE(proposal_content, ''),
		        proposal_target, batch_id, applied_value, undone_at
		 FROM issues WHERE id = ?`, id,
	).Scan(&status, &personaSlug, &proposalType, &proposalContent, &proposalTarget, &batchID, &appliedValue, &undoneAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "no issue with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if status != "done" {
		writeError(w, http.StatusConflict, "wrong_status",
			"undo requires status=done, found "+status)
		return
	}
	if undoneAt.Valid {
		writeError(w, http.StatusConflict, "already_undone",
			"this issue has already been undone")
		return
	}
	if _, ok := undoableProposalTypes[proposalType]; !ok {
		writeError(w, http.StatusUnprocessableEntity, "not_undoable",
			"proposal_type "+proposalType+" cannot be undone")
		return
	}

	target := map[string]any{}
	if proposalTarget.Valid && proposalTarget.String != "" {
		if err := json.Unmarshal([]byte(proposalTarget.String), &target); err != nil {
			writeError(w, http.StatusInternalServerError, "bad_target", err.Error())
			return
		}
	}

	// Variable-product undo: dedicated path. The simple-product
	// buildUndoParams returns one (field, value) pair; we need many.
	if proposalType == "product_price_change_variable" {
		s.handleUndoVariablePriceChange(w, r, id, target, appliedValue.String, personaSlug, batchID)
		return
	}

	pid, err := requireIntFromTarget(target, "product_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", err.Error())
		return
	}

	undoField, undoValue, perr := buildUndoParams(proposalType, target)
	if perr != nil {
		writeError(w, perr.HTTPStatus, perr.Code, perr.Message)
		return
	}

	persona := manifest.PersonaMarketing
	if personaSlug.Valid && strings.TrimSpace(personaSlug.String) != "" {
		persona = manifest.Persona(personaSlug.String)
	}
	batchIDStr := ""
	if batchID.Valid {
		batchIDStr = batchID.String
	}

	// 1) Staleness check via wooagent-products/get. Compare the live
	// value of the field we're about to overwrite against
	// issues.applied_value (what we wrote at approve time). If they
	// differ, someone changed it after our approval — bail with a typed
	// 409 so the operator handles it by hand. Pre-migration approvals
	// (applied_value=NULL) skip the check; in practice the UI won't show
	// Undo for those, but the server stays permissive rather than
	// blocking legitimate undo.
	if appliedValue.Valid {
		current, gerr := fetchCurrentFieldValue(ctx, s.pep, persona, pid, undoField, id, batchIDStr)
		if gerr != nil {
			writeError(w, http.StatusBadGateway, "stale_check_failed", gerr.Error())
			return
		}
		if !equalsCanonical(current, appliedValue.String, undoField) {
			// Canonical {"error": {...}} envelope plus a `current` field
			// inside it so the UI's ApiError parser picks up the live
			// value without needing a special-case shape.
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": map[string]any{
					"code":    "undo_stale",
					"message": "product was changed after this approval — inspect it in WooCommerce",
					"current": current,
				},
			})
			return
		}
	}

	// 2) Reverse write via PEP — same ability + persona scope as the
	// approve path. The audit row carries issue_id so forensic queries
	// can pair the original approve with its undo.
	params := map[string]any{"id": pid, undoField: undoValue}
	decision, _, invokeErr := s.pep.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: "wooagent-products/update",
		Args:    params,
		Intent:  pep.IntentApply,
		Source:  pep.SourceOperator,
		IssueID: id,
		BatchID: batchIDStr,
	})
	if !decision.Allowed {
		if invokeErr != nil {
			if errors.Is(invokeErr, pep.ErrMCPNotConfigured) {
				writeError(w, http.StatusServiceUnavailable, "mcp_not_configured",
					"daemon started without MCP credentials — set WOOAGENT_MCP_URL/USER/APP_PASSWORD and restart")
				return
			}
			writeError(w, http.StatusBadGateway, "mcp_call_failed", invokeErr.Error())
			return
		}
		writePEPDenial(w, decision.Reason)
		return
	}

	// 3) Stamp undone_at idempotently. status stays 'done' — the
	// operator's verdict (approve) wasn't retracted; they're saying
	// "and also, please put it back."
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET undone_at = ?, updated_at = ? WHERE id = ? AND undone_at IS NULL`,
		now, now, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeError(w, http.StatusConflict, "already_undone",
			"this issue was undone concurrently")
		return
	}

	_ = telemetry.RecordVerdict(ctx, s.store.DB, id, telemetry.Verdict{
		Kind:      telemetry.VerdictUndo,
		DecidedAt: time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"status":     "done",
		"undone_at":  now,
		"ability":    "wooagent-products/update",
		"audit_id":   decision.AuditID,
		"updated_at": now,
	})
}

// handleUndoVariablePriceChange reverses a product_price_change_variable
// approval. Fetches live per-variation prices, compares each against the
// applied_value snapshot for drift, then writes the per-variation
// previous_price back via wooagent-products/update-variations-bulk in a
// single call.
func (s *Server) handleUndoVariablePriceChange(
	w http.ResponseWriter,
	r *http.Request,
	issueID string,
	target map[string]any,
	appliedValueRaw string,
	personaSlug sql.NullString,
	batchID sql.NullString,
) {
	ctx := r.Context()

	parentIDf, ok := target["product_id"].(float64)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", "missing or non-numeric product_id")
		return
	}
	parentID := int(parentIDf)

	rawVariations, ok := target["variations"].([]any)
	if !ok || len(rawVariations) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", "missing or empty variations[] in target")
		return
	}

	// Build the reverse-write list from target.variations[].previous_price.
	// We don't trust appliedValueRaw alone — previous_price lives in the
	// target and is the authoritative undo value.
	type reverseEntry struct {
		variationID int
		field       string
		prevValue   string
	}
	reverse := make([]reverseEntry, 0, len(rawVariations))
	for i, raw := range rawVariations {
		v, ok := raw.(map[string]any)
		if !ok {
			writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", fmt.Sprintf("variations[%d] is not an object", i))
			return
		}
		vidF, ok := v["variation_id"].(float64)
		if !ok {
			writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", fmt.Sprintf("variations[%d].variation_id missing", i))
			return
		}
		field := "regular_price"
		if fs, present := v["target_field"]; present {
			s, ok := fs.(string)
			if !ok {
				writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", fmt.Sprintf("variations[%d].target_field not string", i))
				return
			}
			s = strings.TrimSpace(s)
			switch s {
			case "regular_price", "sale_price":
				field = s
			case "":
				// back-compat
			default:
				writeError(w, http.StatusUnprocessableEntity, "bad_proposal_target", fmt.Sprintf("variations[%d] invalid target_field %q", i, s))
				return
			}
		}
		prev, present := v["previous_price"]
		if !present {
			writeError(w, http.StatusUnprocessableEntity, "no_prior_value", fmt.Sprintf("variations[%d] has no previous_price; cannot undo", i))
			return
		}
		var prevF float64
		switch n := prev.(type) {
		case float64:
			prevF = n
		case string:
			parsed, perr := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if perr != nil {
				writeError(w, http.StatusUnprocessableEntity, "no_prior_value", fmt.Sprintf("variations[%d].previous_price not numeric", i))
				return
			}
			prevF = parsed
		default:
			writeError(w, http.StatusUnprocessableEntity, "no_prior_value", fmt.Sprintf("variations[%d].previous_price unexpected type %T", i, prev))
			return
		}
		if prevF <= 0 {
			writeError(w, http.StatusUnprocessableEntity, "no_prior_value", fmt.Sprintf("variations[%d].previous_price must be > 0", i))
			return
		}
		reverse = append(reverse, reverseEntry{
			variationID: int(vidF),
			field:       field,
			prevValue:   strconv.FormatFloat(prevF, 'f', 2, 64),
		})
	}

	persona := manifest.PersonaMarketing
	if personaSlug.Valid && strings.TrimSpace(personaSlug.String) != "" {
		persona = manifest.Persona(personaSlug.String)
	}
	batchIDStr := ""
	if batchID.Valid {
		batchIDStr = batchID.String
	}

	// 1) Drift check: fetch live variations, compare each against the
	// applied snapshot. Skip the check if applied_value is empty
	// (pre-migration approvals).
	if strings.TrimSpace(appliedValueRaw) != "" {
		var snapshot []map[string]any
		if err := json.Unmarshal([]byte(appliedValueRaw), &snapshot); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "bad_applied_value", fmt.Sprintf("applied_value not JSON: %v", err))
			return
		}
		liveByID, gerr := fetchLiveVariationPrices(ctx, s.pep, persona, parentID, issueID, batchIDStr)
		if gerr != nil {
			writeError(w, http.StatusBadGateway, "stale_check_failed", gerr.Error())
			return
		}
		for _, entry := range snapshot {
			vidF, _ := entry["variation_id"].(float64)
			vid := int(vidF)
			field, _ := entry["field"].(string)
			expected, _ := entry["value"].(string)
			live, present := liveByID[vid]
			if !present {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": map[string]any{
						"code":    "undo_stale",
						"message": fmt.Sprintf("variation %d no longer exists or is disabled — inspect in WooCommerce", vid),
					},
				})
				return
			}
			liveVal := live.regularPrice
			if field == "sale_price" {
				liveVal = live.salePrice
			}
			if !equalsCanonical(liveVal, expected, field) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": map[string]any{
						"code":    "undo_stale",
						"message": fmt.Sprintf("variation %d %s was changed after approval — inspect in WooCommerce", vid, field),
						"current": liveVal,
					},
				})
				return
			}
		}
	}

	// 2) Reverse write via PEP.
	updates := make([]map[string]any, 0, len(reverse))
	for _, r := range reverse {
		updates = append(updates, map[string]any{
			"variation_id": r.variationID,
			r.field:        r.prevValue,
		})
	}
	decision, _, invokeErr := s.pep.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: "wooagent-products/update-variations-bulk",
		Args:    map[string]any{"parent_id": parentID, "updates": updates},
		Intent:  pep.IntentApply,
		Source:  pep.SourceOperator,
		IssueID: issueID,
		BatchID: batchIDStr,
	})
	if !decision.Allowed {
		if invokeErr != nil {
			if errors.Is(invokeErr, pep.ErrMCPNotConfigured) {
				writeError(w, http.StatusServiceUnavailable, "mcp_not_configured", "daemon started without MCP credentials")
				return
			}
			writeError(w, http.StatusBadGateway, "mcp_call_failed", invokeErr.Error())
			return
		}
		writePEPDenial(w, decision.Reason)
		return
	}

	// 3) Stamp undone_at.
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.store.DB.ExecContext(ctx,
		`UPDATE issues SET undone_at = ?, updated_at = ? WHERE id = ? AND undone_at IS NULL`,
		now, now, issueID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeError(w, http.StatusConflict, "already_undone", "this issue was undone concurrently")
		return
	}

	_ = telemetry.RecordVerdict(ctx, s.store.DB, issueID, telemetry.Verdict{
		Kind:      telemetry.VerdictUndo,
		DecidedAt: time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         issueID,
		"status":     "done",
		"undone_at":  now,
		"ability":    "wooagent-products/update-variations-bulk",
		"audit_id":   decision.AuditID,
		"updated_at": now,
	})
}

// livePrice holds the regular_price + sale_price strings for one variation.
type livePrice struct {
	regularPrice string
	salePrice    string
}

// fetchLiveVariationPrices calls wooagent-products/variations-list and
// returns a map of variation_id → live prices for drift detection.
func fetchLiveVariationPrices(
	ctx context.Context,
	p *pep.PEP,
	persona manifest.Persona,
	parentID int,
	issueID, batchID string,
) (map[int]livePrice, error) {
	decision, mcpRes, err := p.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: "wooagent-products/variations-list",
		Args:    map[string]any{"product_id": parentID},
		Intent:  pep.IntentRead,
		Source:  pep.SourceOperator,
		IssueID: issueID,
		BatchID: batchID,
	})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, fmt.Errorf("PEP denied wooagent-products/variations-list: %s", decision.Reason)
	}
	if len(mcpRes.Content) == 0 {
		return nil, fmt.Errorf("variations-list returned empty content")
	}
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(mcpRes.Content[0].Text), &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if !env.Success {
		return nil, fmt.Errorf("variations-list not successful")
	}
	var data struct {
		ParentID   int `json:"parent_id"`
		Variations []struct {
			ID           int    `json:"id"`
			RegularPrice string `json:"regular_price"`
			SalePrice    string `json:"sale_price"`
		} `json:"variations"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	out := make(map[int]livePrice, len(data.Variations))
	for _, v := range data.Variations {
		out[v.ID] = livePrice{regularPrice: v.RegularPrice, salePrice: v.SalePrice}
	}
	return out, nil
}

// buildUndoParams derives the (field, value) pair we need to write to
// reverse an approve. For price changes, the field comes from
// target.target_field (defaulting to regular_price for back-compat with
// proposals that predate the sale-price work) and the value is the
// formatted previous_price. For description rewrites, the field is
// "description" and the value is target.previous (the body the product
// had before we shipped the rewrite).
func buildUndoParams(proposalType string, target map[string]any) (field string, value string, err *undoParamsErr) {
	switch proposalType {
	case "product_price_change":
		field = "regular_price"
		if v, ok := target["target_field"].(string); ok {
			v = strings.TrimSpace(v)
			switch v {
			case "regular_price", "sale_price":
				field = v
			case "":
				// fall through: default already set
			default:
				return "", "", &undoParamsErr{
					HTTPStatus: http.StatusUnprocessableEntity,
					Code:       "bad_proposal_target",
					Message:    "invalid target_field " + v,
				}
			}
		}
		prev, present := target["previous_price"]
		if !present {
			return "", "", &undoParamsErr{
				HTTPStatus: http.StatusUnprocessableEntity,
				Code:       "no_prior_value",
				Message:    "proposal target has no previous_price; cannot undo",
			}
		}
		var f float64
		switch n := prev.(type) {
		case float64:
			f = n
		case string:
			parsed, perr := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if perr != nil {
				return "", "", &undoParamsErr{HTTPStatus: http.StatusUnprocessableEntity, Code: "no_prior_value", Message: "previous_price is not numeric"}
			}
			f = parsed
		default:
			return "", "", &undoParamsErr{HTTPStatus: http.StatusUnprocessableEntity, Code: "no_prior_value", Message: fmt.Sprintf("previous_price has unexpected type %T", prev)}
		}
		if f <= 0 {
			return "", "", &undoParamsErr{HTTPStatus: http.StatusUnprocessableEntity, Code: "no_prior_value", Message: "previous_price must be > 0"}
		}
		value = strconv.FormatFloat(f, 'f', 2, 64)
		return field, value, nil

	case "product_description_rewrite":
		prev, _ := target["previous"].(string)
		if strings.TrimSpace(prev) == "" {
			return "", "", &undoParamsErr{
				HTTPStatus: http.StatusUnprocessableEntity,
				Code:       "no_prior_value",
				Message:    "proposal target has no previous description; cannot undo",
			}
		}
		return "description", prev, nil

	default:
		return "", "", &undoParamsErr{
			HTTPStatus: http.StatusUnprocessableEntity,
			Code:       "not_undoable",
			Message:    "proposal_type " + proposalType + " cannot be undone",
		}
	}
}

// fetchCurrentFieldValue calls wooagent-products/get via PEP and returns
// the named field's current value as a string. Used by the staleness
// check.
func fetchCurrentFieldValue(
	ctx context.Context,
	p *pep.PEP,
	persona manifest.Persona,
	productID int,
	field, issueID, batchID string,
) (string, error) {
	decision, mcpRes, err := p.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: "wooagent-products/get",
		Args:    map[string]any{"id": productID},
		Intent:  pep.IntentRead,
		Source:  pep.SourceOperator,
		IssueID: issueID,
		BatchID: batchID,
	})
	if err != nil {
		return "", err
	}
	if !decision.Allowed {
		return "", fmt.Errorf("PEP denied wooagent-products/get: %s", decision.Reason)
	}
	if len(mcpRes.Content) == 0 {
		return "", fmt.Errorf("wooagent-products/get returned empty content")
	}
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(mcpRes.Content[0].Text), &env); err != nil {
		return "", fmt.Errorf("decode envelope: %w", err)
	}
	if !env.Success {
		return "", fmt.Errorf("wooagent-products/get not successful")
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	switch v := data[field].(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', 2, 64), nil
	case nil:
		return "", nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// equalsCanonical compares a live field value to our recorded applied
// value, tolerating Woo's whitespace and decimal-formatting quirks for
// price fields (where "44.99" and "44.9900" and " 44.99 " all mean the
// same thing). For description fields it's a byte-for-byte compare
// after TrimSpace on both sides — Woo doesn't re-format descriptions.
func equalsCanonical(live, applied, field string) bool {
	switch field {
	case "regular_price", "sale_price":
		l, lerr := strconv.ParseFloat(strings.TrimSpace(live), 64)
		a, aerr := strconv.ParseFloat(strings.TrimSpace(applied), 64)
		if lerr != nil || aerr != nil {
			return strings.TrimSpace(live) == strings.TrimSpace(applied)
		}
		return strconv.FormatFloat(l, 'f', 2, 64) == strconv.FormatFloat(a, 'f', 2, 64)
	default:
		return strings.TrimSpace(live) == strings.TrimSpace(applied)
	}
}
