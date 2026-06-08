// Package personas is the in-process registry for the daemon's agent
// personas (Marketing, Pricing, Sales Support, …). Each persona lives in a
// child package that side-effect-registers itself via personas.Register in
// init(). The daemon imports the child packages for their side effect
// (`import _ ".../personas/marketing"`), then iterates personas.All() at
// startup to fan out work in goroutines.
//
// Two contracts:
//
//   - Persona.Draft is pure-ish — it does the read+model work and returns a
//     proposal payload, but doesn't persist. Pure-ish because it does talk
//     to MCP and external LLM APIs; what it doesn't do is mutate the store
//     or HTTP-broadcast. Both Skipped and Error returns must be explainable
//     in one operator-readable sentence.
//   - personas.RunAndPersist is the boot-time wrapper: it checks the
//     agents-table enabled flag, guards against duplicate seeding, calls
//     Draft, and inserts the issue. Persistence is centralized here so
//     future personas don't each reinvent the SQL.
package personas

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/registry"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// Persona is what a child package implements. Slug must match the value in
// the agents table (and in manifest.PersonaXxx). DisplayName is operator-
// readable. Draft does the actual work. Cooldown reports the persona's
// dedup policy so the picker can avoid re-proposing on a recently-touched
// target (a product, an order, etc.).
//
// When adding a new persona (Inventory, Accounting, Reporting, Chief of
// Staff, …), apply ALL THREE established patterns:
//
//   1. Implement Cooldown() with the right TargetKey + per-risk durations.
//      Customer-facing targets (orders, messages) → longer windows; pure
//      back-of-house copy/price tweaks → shorter windows. Call
//      RecentlyTouchedTargets at the top of Draft and pass the set into
//      the picker. Digest-style personas with no per-target id leave
//      Cooldown zero-value and rely on DedupKey (pattern 3) instead.
//
//   2. Run an in-Draft loop up to maxDraftAttempts (=3, defined per
//      package). If the LLM returns no_proposal / empty draft for the
//      first pick, add that target to a RUN-LOCAL skip set and try the
//      next eligible one. Without this, an undraftable target (e.g. a
//      product the web_search can't find comparables for) blocks the
//      persona indefinitely — the failed target never enters the
//      persistent cooldown because no issue was ever inserted.
//
//   3. Set Drafted.DedupKey to a stable string identifying the proposal's
//      logical target so the system-wide insert-time guard refuses to
//      emit a second in_review issue with the same identity. Marketing /
//      Pricing use "product:<id>"; Sales Support uses "order:<id>";
//      digest personas use "digest:<kind>" (add an ISO-week suffix for
//      time-bucketed digests).
//
// See marketing.go / pricing.go / sales-support.go for canonical examples.
type Persona interface {
	Slug() string
	DisplayName() string
	Cooldown() CooldownPolicy
	Draft(ctx context.Context, deps Deps) (Drafted, error)
	// Addable reports whether this persona ships dormant — i.e. it's
	// implemented but not enabled by default. Operators opt addable
	// personas in via the Add Agent modal. Default-on personas
	// (Marketing, Pricing, Sales Support) return false; personas that
	// need explicit opt-in (Reporting today) return true.
	Addable() bool
}

// CooldownPolicy is the per-persona dedup config consumed by
// RecentlyTouchedTargets. TargetKey is the JSON key inside proposal_target
// whose integer value identifies the thing the persona just proposed on —
// "product_id" for product-centric personas (Marketing, Pricing), or
// "order_id" for order-centric ones (Sales Support). Approved is how long
// to suppress that target after an approve (status='done'); Dismissed is
// how long after a dismiss (status='dismissed'). Different personas can
// pick different windows: Sales Support's customer-facing messages carry
// more risk than a product description rewrite, so we cool down longer.
type CooldownPolicy struct {
	TargetKey string
	Approved  time.Duration
	Dismissed time.Duration
	Skipped   time.Duration // cooldown after an LLM-level skip; 0 disables
}

// Deps is everything a persona is allowed to reach for. Adding new fields
// here is purely additive; personas only read what they need. Personas
// MUST NOT touch process env directly — Env is the surfaced subset.
type Deps struct {
	Store    *store.Store
	MCP      *mcp.Client
	Skills   map[string]registry.Skill
	Env      Env
	// Recorder persists the per-turn telemetry. May be nil in tests; the
	// tracker helpers are nil-safe. DSGWOO-1236.
	Recorder telemetry.Recorder
	// MaxEmits caps how many proposals a single RunAndPersist call should
	// produce. Zero (the default) is treated as 1 — the historical
	// single-emit-per-run behavior. The scheduler sets this to 2 on
	// TriggerBootstrap so the operator's queue is non-empty right after
	// onboarding completes. Subsequent emits within the same run rely on
	// the just-inserted issue showing up in RecentlyTouchedTargets so
	// each persona picks a *different* target each loop.
	MaxEmits int
	// Abilities answers "is this fully-qualified ability invokable on the
	// connected store right now?" Personas use it to branch between a
	// baseline path (always-on Companion Plugin abilities) and an enhanced
	// path that depends on an optional plugin (e.g. the WC AI plugin's
	// woocommerce/find-products, woocommerce/plan-batch-operation, etc.).
	//
	// Nil-safe: when unset, personas should fall through to the baseline
	// path (NilAbilities is the explicit zero value for tests and debug
	// runs).
	Abilities Abilities
	// RunBudgetCents is the per-run model-cost cap in cents, read by the
	// scheduler from agents.run_budget_cents. Zero (the default) disables
	// enforcement — tests and debug runs that don't set it never trip the
	// gate. When non-zero, RunAndPersist passes it to the per-turn
	// telemetry.Tracker, which returns ErrRunBudgetExceeded on the model
	// call that pushes total cost over the cap. DSGWOO-1296.
	RunBudgetCents int64
}

// Abilities reports which ability names are presently invokable on the
// connected store. "Invokable" mirrors the PEP's trust-gate semantics:
// the ability is either pre-signed in the bundled manifest, or operator-
// trusted in the abilities cache at its current schema_hash, and not
// revoked. A persona asking Has does not get a hard guarantee that an
// eventual call will succeed (PEP runs more checks: persona scope, schema
// validation, budget); it gets the lightweight pre-check needed to decide
// which code path to take.
type Abilities interface {
	Has(name string) bool
}

// NilAbilities is the explicit "no enhanced abilities available" sentinel.
// Personas constructed with NilAbilities always take the baseline path.
type NilAbilities struct{}

// Has always returns false.
func (NilAbilities) Has(string) bool { return false }

// Env is the env-var surface area a persona is allowed to consult. We
// flatten it through Deps so unit tests can supply fixtures and so missing
// envs surface as "skipped, set X" rather than os.Getenv reads scattered
// across packages.
type Env struct {
	AnthropicAPIKey   string
	AnthropicModel    string
	OpenAIAPIBase     string
	OpenAIAPIKey      string
	OpenAIModel       string
	DefaultCurrency   string // e.g. "USD"; pricing falls back to this when the store doesn't surface a currency
	ProductIDOverride int    // PERSONA_PRODUCT_ID — forwarded for debug runs
}

// Drafted is the persona's output, ready to land as an issue. The Skipped
// branch is a first-class outcome (insufficient grounding, missing env,
// nothing to propose). Skipped must come with a human-readable reason.
type Drafted struct {
	Title           string
	Description     string
	Priority        string // "low" | "medium" | "high" | "urgent" | "none"
	ProposalType    string
	ProposalContent string
	Target          map[string]any
	Skipped         bool
	SkipReason      string

	// DedupKey is the proposal's logical identity for the insert-time
	// guard in RunAndPersist. When non-empty, a new emit is skipped if an
	// in_review issue with the same (persona, dedup_key) already exists.
	// Empty string opts out — back-compat default. Examples:
	//   Marketing/Pricing: "product:<id>"
	//   Sales Support:     "order:<id>"
	//   Reporting:         "digest:data_issues" (or "digest:<kind>:<period>"
	//                      for time-bucketed digests)
	// Cooldown still owns the approved/dismissed history windows; this
	// field only blocks while a prior proposal is still open.
	DedupKey string

	// BatchSiblings, when non-nil, turns this Drafted into the *first* child
	// of a batch. RunAndPersist creates a row in the `batches` table and
	// attaches this Drafted plus each sibling as a child issue, all sharing
	// the new batch_id. Each sibling's own Title / ProposalType / Target /
	// Cooldown is honored. Used by personas that group their scan results
	// (e.g. Pricing emitting a category batch when N>=3 products are flagged).
	BatchSiblings []Drafted

	// BatchTitle is the title of the parent batches row. Required when
	// BatchSiblings is non-nil; ignored otherwise. Example:
	// "Pricing · Home & Textiles seasonal parity run (11 products)".
	BatchTitle string

	// BatchIntent is the batches.intent column value (e.g. "pricing_bulk").
	// Required when BatchSiblings is non-nil; ignored otherwise.
	BatchIntent string
}

// Result is what RunAndPersist returns. With MaxEmits>1, a single run may
// produce multiple issues and/or batches. IssueID/BatchID retain the
// *first* such ID for back-compat with the scheduler runs.issue_id column
// (which still stores a single value); IssueIDs and BatchIDs hold the
// full set in emission order. IssueID is empty when Skipped or when an
// error was returned before any successful emit.
type Result struct {
	Persona  string
	IssueID  string
	BatchID  string
	IssueIDs []string
	BatchIDs []string
	Skipped  bool
	SkipReason string
}

// ---- Registry ----

var (
	regMu sync.RWMutex
	reg   = map[string]Persona{}
)

// Register adds a persona to the global registry. Intended to be called
// from a child package's init(). Duplicate slugs panic — registration is a
// build-time decision, not a runtime one.
func Register(p Persona) {
	if p == nil {
		panic("personas.Register: nil persona")
	}
	slug := p.Slug()
	if slug == "" {
		panic("personas.Register: empty slug")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := reg[slug]; exists {
		panic(fmt.Sprintf("personas.Register: duplicate slug %q", slug))
	}
	reg[slug] = p
}

// All returns every registered persona. The slice is a snapshot; iteration
// order is not stable.
func All() []Persona {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Persona, 0, len(reg))
	for _, p := range reg {
		out = append(out, p)
	}
	return out
}

// Lookup returns the persona registered under slug. Used by the CLI debug
// binaries (cmd/persona-<slug>/) so a single binary can route to the right
// persona by name.
func Lookup(slug string) (Persona, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := reg[slug]
	return p, ok
}

// ---- Lifecycle helpers ----

// IsEnabled reads agents.enabled for slug. Returns false if the row is
// missing (treat unseeded personas as disabled).
func IsEnabled(ctx context.Context, st *store.Store, slug string) (bool, error) {
	var enabled int
	err := st.DB.QueryRowContext(ctx,
		`SELECT enabled FROM agents WHERE persona = ?`, slug,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return enabled == 1, nil
}

// OpenProposalSkipThreshold is the inclusive lower bound for "too much
// open work to enqueue another run." A persona is allowed to keep one
// proposal pending review while the next cadence tick still produces a
// fresh draft; the skip only fires when the count reaches this many.
const OpenProposalSkipThreshold = 2

// CountOpenWork returns how many open units of work for slug are currently
// in todo / in_progress / in_review. A "unit" is one stand-alone issue OR
// one batch — a 9-child marketing batch counts as 1, so the operator can
// still kick off a fresh marketing run while the batch is pending review.
// Done and rejected don't count — those are resolved. Callers compare
// against OpenProposalSkipThreshold to decide whether to skip seeding
// (RunAndPersist) or skip a scheduler tick (the Loop's HasOpenWorkFn
// wiring). COALESCE(batch_id, id) folds each batch's children to a single
// bucket while leaving stand-alone issues distinct.
func CountOpenWork(ctx context.Context, st *store.Store, slug string) (int, error) {
	var n int
	err := st.DB.QueryRowContext(ctx,
		`SELECT count(DISTINCT COALESCE(batch_id, id)) FROM issues
		 WHERE persona = ? AND status IN ('todo','in_progress','in_review')`,
		slug,
	).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// RecentlyTouchedTargets returns the set of integer target ids the
// persona should skip when picking the next thing to propose on, per the
// provided CooldownPolicy. A target is in the set if persona has any:
//   - open issue on it (todo / in_progress / in_review), or
//   - approved issue on it (status='done')      within policy.Approved, or
//   - dismissed issue on it (status='dismissed') within policy.Dismissed.
//
// The target id is read from proposal_target JSON via json_extract on
// policy.TargetKey (e.g. "product_id" or "order_id"); rows without a
// numeric value for that key are ignored. Returns an empty (non-nil) map
// when nothing is in cooldown. Returns an error if policy.TargetKey is
// empty — a typo there would silently match every row.
func RecentlyTouchedTargets(
	ctx context.Context,
	st *store.Store,
	slug string,
	policy CooldownPolicy,
) (map[int]struct{}, error) {
	if policy.TargetKey == "" {
		return nil, fmt.Errorf("RecentlyTouchedTargets: policy.TargetKey is empty")
	}
	now := time.Now().UTC()
	approvedCutoff := now.Add(-policy.Approved).Format(time.RFC3339)
	dismissedCutoff := now.Add(-policy.Dismissed).Format(time.RFC3339)
	// json_extract path is built from the trusted persona-declared key, not
	// from any operator/network input — concatenation here is safe.
	jsonPath := "$." + policy.TargetKey

	rows, err := st.DB.QueryContext(ctx, `
		SELECT DISTINCT CAST(json_extract(proposal_target, ?) AS INTEGER) AS tid
		FROM issues
		WHERE persona = ?
		  AND json_extract(proposal_target, ?) IS NOT NULL
		  AND (
		        status IN ('todo','in_progress','in_review')
		     OR (status = 'done'      AND updated_at   > ?)
		     OR (status = 'dismissed' AND dismissed_at > ?)
		  )`,
		jsonPath, slug, jsonPath, approvedCutoff, dismissedCutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("query recently-touched targets: %w", err)
	}
	defer rows.Close()

	out := make(map[int]struct{})
	for rows.Next() {
		var tid sql.NullInt64
		if err := rows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("scan target id: %w", err)
		}
		if tid.Valid && tid.Int64 > 0 {
			out[int(tid.Int64)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate target ids: %w", err)
	}
	return out, nil
}

// RecentlySkippedTargets returns the set of integer target ids the persona
// LLM-skipped (examined and declined at draft time) within policy.Skipped of
// now. These are merged into the picker's skip set so a recently-declined
// target drops out of contention until its cooldown lapses. Returns an empty
// (non-nil) map when policy.Skipped == 0 (feature off) or nothing qualifies.
func RecentlySkippedTargets(
	ctx context.Context,
	st *store.Store,
	slug string,
	policy CooldownPolicy,
) (map[int]struct{}, error) {
	out := make(map[int]struct{})
	if policy.Skipped == 0 {
		return out, nil
	}
	cutoff := time.Now().UTC().Add(-policy.Skipped).Format(time.RFC3339)

	rows, err := st.DB.QueryContext(ctx, `
		SELECT target_id
		FROM llm_skips
		WHERE persona = ?
		  AND attempted_at > ?`,
		slug, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("query recently-skipped targets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tid sql.NullInt64
		if err := rows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("scan skip target id: %w", err)
		}
		if tid.Valid && tid.Int64 > 0 {
			out[int(tid.Int64)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate skip target ids: %w", err)
	}
	return out, nil
}

// RecordLLMSkip upserts an LLM-level skip for (slug, targetID): the persona
// examined the target at draft time and declined to propose. Re-recording the
// same (persona, targetID) refreshes attempted_at and skip_reason, restarting
// the Skipped cooldown clock. now is passed in for testability; callers use
// time.Now().UTC().
func RecordLLMSkip(
	ctx context.Context,
	st *store.Store,
	slug string,
	targetID int,
	reason string,
	now time.Time,
) error {
	_, err := st.DB.ExecContext(ctx, `
		INSERT INTO llm_skips(persona, target_id, skip_reason, attempted_at)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(persona, target_id)
		DO UPDATE SET skip_reason = excluded.skip_reason,
		              attempted_at = excluded.attempted_at`,
		slug, targetID, reason, now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("record llm skip (%s/%d): %w", slug, targetID, err)
	}
	return nil
}

// findOpenIssueWithDedupKey returns the id of an in_review issue for the
// given persona whose dedup_key matches the supplied key, or an empty
// string when none exists. NULL-stored keys never match (the partial
// index excludes them anyway). Used by RunAndPersist's insert-time guard
// to skip a Drafted whose logical identity is already represented by an
// open proposal.
func findOpenIssueWithDedupKey(
	ctx context.Context,
	st *store.Store,
	slug, key string,
) (string, error) {
	var id sql.NullString
	err := st.DB.QueryRowContext(ctx, `
		SELECT id FROM issues
		 WHERE persona = ?
		   AND dedup_key = ?
		   AND status = 'in_review'
		 LIMIT 1`,
		slug, key,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query open dedup issue: %w", err)
	}
	if !id.Valid {
		return "", nil
	}
	return id.String, nil
}

// RunAndPersist is the boot-time entry point. Checks agents.enabled,
// guards against duplicate seeding, calls Draft, and inserts the issue.
// All persistence happens here so child packages stay focused on the
// MCP+LLM work.
//
// When deps.MaxEmits > 1, RunAndPersist loops over p.Draft repeatedly,
// inserting after each successful emit. Subsequent iterations naturally
// pick a *different* target because the just-inserted issue ends up in
// RecentlyTouchedTargets' cooldown set (in_review counts as "open"). A
// hard error or skip after the first successful emit ends the loop
// best-effort: prior emits are kept; the unsuccessful tail is logged but
// doesn't fail the run.
func RunAndPersist(ctx context.Context, p Persona, deps Deps) (Result, error) {
	slug := p.Slug()
	res := Result{Persona: slug}

	enabled, err := IsEnabled(ctx, deps.Store, slug)
	if err != nil {
		return res, fmt.Errorf("check enabled: %w", err)
	}
	if !enabled {
		res.Skipped = true
		res.SkipReason = "persona not enabled in agents table"
		return res, nil
	}
	openCount, err := CountOpenWork(ctx, deps.Store, slug)
	if err != nil {
		return res, fmt.Errorf("check open work: %w", err)
	}
	if openCount >= OpenProposalSkipThreshold {
		res.Skipped = true
		res.SkipReason = fmt.Sprintf(
			"%d proposals already waiting for review",
			openCount,
		)
		return res, nil
	}

	emitTarget := deps.MaxEmits
	if emitTarget < 1 {
		emitTarget = 1
	}

	for i := 0; i < emitTarget; i++ {
		// One tracker per Draft call so each emitted issue gets its own
		// turn_event row in the GEPA pipeline.
		tracker := telemetry.NewTracker(uuid.NewString(), slug)
		// Per-run model-cost cap. Zero disables enforcement so tests and
		// debug runs that don't set RunBudgetCents keep working. DSGWOO-1296.
		if deps.RunBudgetCents > 0 {
			tracker.SetBudgetUSD(float64(deps.RunBudgetCents) / 100.0)
		}
		tctx := telemetry.WithTracker(ctx, tracker)

		d, err := p.Draft(tctx, deps)
		if err != nil {
			recordTurn(ctx, deps.Recorder, tracker, "", d)
			if i == 0 {
				return res, err
			}
			// Best-effort: keep prior emits, surface the partial failure on
			// the run row.
			if res.SkipReason == "" {
				res.SkipReason = fmt.Sprintf("emit %d/%d failed: %v", i+1, emitTarget, err)
			}
			break
		}
		if d.Skipped {
			recordTurn(ctx, deps.Recorder, tracker, "", d)
			if i == 0 {
				res.Skipped = true
				res.SkipReason = d.SkipReason
				return res, nil
			}
			// Best-effort: prior emits stand.
			if res.SkipReason == "" {
				res.SkipReason = fmt.Sprintf("emit %d/%d skipped: %s", i+1, emitTarget, d.SkipReason)
			}
			break
		}

		// Batch branch: if the persona returned BatchSiblings, persist a
		// single batches row + N+1 child issues atomically. Each child
		// shares the new batch_id and lands as its own in_review issue, so
		// the next loop iteration's RecentlyTouchedTargets correctly
		// excludes every product/order in the batch.
		if len(d.BatchSiblings) > 0 {
			if d.BatchTitle == "" || d.BatchIntent == "" {
				recordTurn(ctx, deps.Recorder, tracker, "", d)
				if i == 0 {
					return res, fmt.Errorf("BatchSiblings set but BatchTitle or BatchIntent empty")
				}
				break
			}
			batchID, err := insertBatch(ctx, deps.Store, slug, d)
			if err != nil {
				recordTurn(ctx, deps.Recorder, tracker, "", d)
				if i == 0 {
					return res, fmt.Errorf("insert batch: %w", err)
				}
				if res.SkipReason == "" {
					res.SkipReason = fmt.Sprintf("emit %d/%d batch insert failed: %v", i+1, emitTarget, err)
				}
				break
			}
			// Telemetry: issueID stays empty for batch turns — the run
			// isn't tied to a single issue row when a batch is produced.
			recordTurn(ctx, deps.Recorder, tracker, "", d)
			res.BatchIDs = append(res.BatchIDs, batchID)
			if res.BatchID == "" {
				res.BatchID = batchID
			}
			continue
		}

		// Insert-time dedup guard: when the persona declared a stable
		// logical identity via Drafted.DedupKey, refuse to emit a second
		// in_review issue with the same key. Cooldown still owns the
		// approved/dismissed history; this only blocks while a prior
		// proposal is still open. Fail-open on a transient query error
		// (the guard is a safety net, not a correctness invariant).
		if d.DedupKey != "" {
			if existingID, qerr := findOpenIssueWithDedupKey(ctx, deps.Store, slug, d.DedupKey); qerr == nil && existingID != "" {
				recordTurn(ctx, deps.Recorder, tracker, "", d)
				reason := fmt.Sprintf("duplicate of %s (still in review)", existingID)
				if i == 0 {
					res.Skipped = true
					res.SkipReason = reason
					return res, nil
				}
				if res.SkipReason == "" {
					res.SkipReason = fmt.Sprintf("emit %d/%d skipped: %s", i+1, emitTarget, reason)
				}
				break
			}
		}

		id, err := insertIssue(ctx, deps.Store, slug, d)
		if err != nil {
			recordTurn(ctx, deps.Recorder, tracker, "", d)
			if i == 0 {
				return res, fmt.Errorf("insert issue: %w", err)
			}
			if res.SkipReason == "" {
				res.SkipReason = fmt.Sprintf("emit %d/%d issue insert failed: %v", i+1, emitTarget, err)
			}
			break
		}
		recordTurn(ctx, deps.Recorder, tracker, id, d)
		res.IssueIDs = append(res.IssueIDs, id)
		if res.IssueID == "" {
			res.IssueID = id
		}
	}

	return res, nil
}

// recordTurn finalizes the tracker and persists the TurnEvent. Best-effort
// — a recorder failure is logged but does not abort the persona run.
// Called from every exit path of RunAndPersist so skipped/errored turns
// still leave a row behind for the GEPA pipeline.
func recordTurn(
	ctx context.Context,
	rec telemetry.Recorder,
	tracker *telemetry.Tracker,
	issueID string,
	d Drafted,
) {
	if rec == nil || tracker == nil {
		return
	}
	event := tracker.Finalize()
	event.IssueID = issueID
	if d.ProposalContent != "" {
		event.ProposalText = d.ProposalContent
	}
	// Skipped turns still write a row so the GEPA pipeline sees the
	// "agent decided not to propose" signal — captured via an empty
	// proposal + the skill/model calls that preceded the skip.
	_ = rec.Record(ctx, event)
}

func insertIssue(ctx context.Context, st *store.Store, persona string, d Drafted) (string, error) {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	priority := d.Priority
	if priority == "" {
		priority = "medium"
	}

	var targetJSON sql.NullString
	if d.Target != nil {
		b, err := json.Marshal(d.Target)
		if err != nil {
			return "", fmt.Errorf("marshal target: %w", err)
		}
		targetJSON = sql.NullString{String: string(b), Valid: true}
	}

	var dedupKey sql.NullString
	if d.DedupKey != "" {
		dedupKey = sql.NullString{String: d.DedupKey, Valid: true}
	}

	_, err := st.DB.ExecContext(ctx,
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target, dedup_key) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, d.Title, d.Description, persona, "in_review", priority, now, now,
		d.ProposalType, d.ProposalContent, targetJSON, dedupKey,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// insertBatch creates one batches row and N+1 child issues in a single
// transaction. The primary Drafted becomes the first child; each entry of
// d.BatchSiblings becomes another. All children share the returned batch
// id. Atomicity matters here — a partially-inserted batch (some children
// but not others) would surface in the UI as an undercount, so we wrap the
// whole thing in BEGIN/COMMIT and let the deferred Rollback unwind on any
// failure.
func insertBatch(ctx context.Context, st *store.Store, persona string, d Drafted) (string, error) {
	batchID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// TODO(run-id): batches.source_run_id is the chain-of-identity to the
	// run that generated the batch (per migration 004's comment). Not yet
	// plumbed through RunAndPersist's call sites — passing "" until a
	// follow-up wires the run id through Deps.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO batches(id, title, persona, intent, source_run_id, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		batchID, d.BatchTitle, persona, d.BatchIntent, "", now, now,
	); err != nil {
		return "", fmt.Errorf("insert batches: %w", err)
	}

	// Primary Drafted is the first child; siblings follow.
	children := make([]Drafted, 0, 1+len(d.BatchSiblings))
	children = append(children, d)
	children = append(children, d.BatchSiblings...)

	for i, child := range children {
		if err := insertChildIssue(ctx, tx, batchID, persona, child, now); err != nil {
			return "", fmt.Errorf("insert child %d/%d: %w", i+1, len(children), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return batchID, nil
}

// insertChildIssue inserts a single issue row tagged with batch_id. Mirrors
// insertIssue's column shape (priority defaults to "medium", target is
// nil-safe via sql.NullString) so unbatched and batched children look
// identical to downstream readers — the only difference is the batch_id
// column.
func insertChildIssue(ctx context.Context, tx *sql.Tx, batchID string, persona string, d Drafted, now string) error {
	id := uuid.NewString()
	priority := d.Priority
	if priority == "" {
		priority = "medium"
	}

	var targetJSON sql.NullString
	if d.Target != nil {
		b, err := json.Marshal(d.Target)
		if err != nil {
			return fmt.Errorf("marshal target: %w", err)
		}
		targetJSON = sql.NullString{String: string(b), Valid: true}
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target, batch_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, d.Title, d.Description, persona, "in_review", priority, now, now,
		d.ProposalType, d.ProposalContent, targetJSON, batchID,
	)
	if err != nil {
		return fmt.Errorf("exec insert: %w", err)
	}
	return nil
}

// ---- Within-run iteration ----

// PickerFunc returns the next target id to try, or an error if no
// eligible target remains. Implementations should consult the provided
// skip set so a target marked off in a previous iteration is not
// re-picked. The skip set is owned by IterateDraft — implementations
// must not retain references to it across calls.
type PickerFunc func(skip map[int]struct{}) (targetID int, err error)

// DrafterFunc performs the per-target work (MCP fetch, LLM call,
// validation). Returning Drafted{Skipped: true} signals an LLM-level
// skip that should advance the iterator to the next target. Returning a
// non-nil error signals a hard failure that should fail the run
// immediately.
type DrafterFunc func(targetID int) (Drafted, error)

// IterateDraft runs the pick/draft loop until a target yields a non-
// skipped Drafted or maxAttempts is exhausted. The skip map is mutated
// in place: any target that returns Drafted{Skipped: true} is added so
// the next pickFn call will avoid it. Callers typically seed skip with
// the persistent cooldown set from RecentlyTouchedTargets before calling.
//
// targetName ("product", "order", …) shows up in the cumulative
// SkipReason when every attempt skips, so the operator can read what was
// tried. See marketing.go / pricing.go / sales-support.go for usage.
func IterateDraft(
	maxAttempts int,
	targetName string,
	skip map[int]struct{},
	pickFn PickerFunc,
	draftFn DrafterFunc,
) (Drafted, error) {
	var attempts []string
	for i := 0; i < maxAttempts; i++ {
		id, pickErr := pickFn(skip)
		if pickErr != nil {
			if len(attempts) > 0 {
				return Drafted{
					Skipped:    true,
					SkipReason: fmt.Sprintf("tried %d %ss, none drafted: %s; then: %v", len(attempts), targetName, strings.Join(attempts, "; "), pickErr),
				}, nil
			}
			return Drafted{Skipped: true, SkipReason: pickErr.Error()}, nil
		}
		drafted, draftErr := draftFn(id)
		if draftErr != nil {
			return Drafted{}, draftErr
		}
		if !drafted.Skipped {
			return drafted, nil
		}
		attempts = append(attempts, fmt.Sprintf("%s %d: %s", targetName, id, drafted.SkipReason))
		skip[id] = struct{}{}
	}
	return Drafted{
		Skipped:    true,
		SkipReason: fmt.Sprintf("tried %d %ss, none drafted: %s", maxAttempts, targetName, strings.Join(attempts, "; ")),
	}, nil
}
