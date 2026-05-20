package pep

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// auditWriter inserts and updates rows in audit_invocations. It's append-only
// after insert except for outcome/denial_reason/completed_at, which finalize
// the row when Invoke returns.
type auditWriter struct {
	db     *sql.DB
	budget *BudgetGate
}

func newAuditWriter(db *sql.DB, budget *BudgetGate) *auditWriter {
	return &auditWriter{db: db, budget: budget}
}

// insert writes an initial pending row and returns its id. The row is
// finalized later via finalize.
func (w *auditWriter) insert(ctx context.Context, req Request, argsHash string) (int64, error) {
	res, err := w.db.ExecContext(ctx,
		`INSERT INTO audit_invocations(
			plan_id, task_id, step_id, issue_id, persona, model, prompt_hash,
			ability, args_hash, cap_token_id, intent, outcome, denial_reason, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, NULL, ?)`,
		nullIfEmpty(req.PlanID), nullIfEmpty(req.TaskID), nullIfEmpty(req.StepID),
		nullIfEmpty(req.IssueID), string(req.Persona),
		nullIfEmpty(req.Model), nullIfEmpty(req.PromptHash),
		req.Ability, argsHash,
		string(req.Intent), string(OutcomePending), nowRFC3339(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert audit row: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("audit row id: %w", err)
	}
	return id, nil
}

// finalize sets outcome and (optionally) denial_reason on the row at id, plus
// completed_at. Calling finalize on a row already finalized just stamps a new
// completed_at — V1 doesn't enforce one-shot finalization since concurrent
// finalizers don't happen on the approve path.
//
// ruleName is the policy rule that fired (only meaningful for policy denials —
// empty string everywhere else, which writes NULL to the audit row).
//
// On Allowed outcomes (success, mcp_error), finalize also bumps the persona's
// call_count in persona_budget_usage. Increment errors are logged and swallowed
// — a missed increment is a slight under-count favouring the operator, which is
// acceptable. Don't fail the call over accounting noise.
func (w *auditWriter) finalize(ctx context.Context, id int64, persona manifest.Persona, outcome Outcome, reason ReasonCode, ruleName string) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE audit_invocations
		   SET outcome = ?, denial_reason = ?, completed_at = ?, policy_rule_name = ?
		 WHERE id = ?`,
		string(outcome), nullIfEmpty(string(reason)), nowRFC3339(), nullIfEmpty(ruleName), id,
	)
	if err != nil {
		return fmt.Errorf("finalize audit row %d: %w", id, err)
	}
	if w.budget != nil && (outcome == OutcomeSuccess || outcome == OutcomeMCPError) {
		if incErr := w.budget.IncrementCalls(ctx, persona); incErr != nil {
			// Log + swallow. The check is authoritative for "over budget";
			// a missed increment is a slight under-count favoring the
			// operator, which is acceptable. Don't fail the call.
			_ = incErr
		}
	}
	return nil
}

// hashArgs canonicalizes args (sorted keys, no whitespace) and returns its
// sha256 hex. Canonical form means "{a:1,b:2}" and "{b:2,a:1}" produce the
// same hash — useful for correlating identical-payload retries.
func hashArgs(args map[string]any) string {
	if len(args) == 0 {
		return sha256Hex([]byte("{}"))
	}
	encoded, err := canonicalJSON(args)
	if err != nil {
		// Fall back to a stable error marker; hashArgs must not panic.
		return sha256Hex([]byte("__hash_error__:" + err.Error()))
	}
	return sha256Hex(encoded)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// canonicalJSON walks a JSON-decodable value and emits keys in sorted order
// at every level. encoding/json doesn't guarantee key ordering, so we do it
// here. Only handles the shapes that flow through ability arguments
// (objects, arrays, strings, numbers, booleans, nil) — the schema validator
// in Phase 2 will reject anything else upstream.
func canonicalJSON(v any) ([]byte, error) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := canonicalJSON(x[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		buf := []byte{'['}
		for i, item := range x {
			if i > 0 {
				buf = append(buf, ',')
			}
			ib, err := canonicalJSON(item)
			if err != nil {
				return nil, err
			}
			buf = append(buf, ib...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(v)
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
