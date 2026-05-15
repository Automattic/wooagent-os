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
// Staff, …), apply BOTH established patterns:
//
//   1. Implement Cooldown() with the right TargetKey + per-risk durations.
//      Customer-facing targets (orders, messages) → longer windows; pure
//      back-of-house copy/price tweaks → shorter windows. Call
//      RecentlyTouchedTargets at the top of Draft and pass the set into
//      the picker.
//
//   2. Run an in-Draft loop up to maxDraftAttempts (=3, defined per
//      package). If the LLM returns no_proposal / empty draft for the
//      first pick, add that target to a RUN-LOCAL skip set and try the
//      next eligible one. Without this, an undraftable target (e.g. a
//      product the web_search can't find comparables for) blocks the
//      persona indefinitely — the failed target never enters the
//      persistent cooldown because no issue was ever inserted.
//
// See marketing.go / pricing.go / sales-support.go for canonical examples.
type Persona interface {
	Slug() string
	DisplayName() string
	Cooldown() CooldownPolicy
	Draft(ctx context.Context, deps Deps) (Drafted, error)
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
}

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

// Result is what RunAndPersist returns. IssueID is empty when Skipped or
// when an error was returned.
type Result struct {
	Persona    string
	IssueID    string
	Skipped    bool
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

// HasOpenWork returns true when slug already has any issue in todo /
// in_progress / in_review. Used by RunAndPersist to skip duplicate seeding
// on every daemon restart. Done and rejected don't count — those are
// resolved.
func HasOpenWork(ctx context.Context, st *store.Store, slug string) (bool, error) {
	var n int
	err := st.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM issues WHERE persona = ? AND status IN ('todo','in_progress','in_review')`,
		slug,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
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

// RunAndPersist is the boot-time entry point. Checks agents.enabled,
// guards against duplicate seeding, calls Draft, and inserts the issue.
// All persistence happens here so child packages stay focused on the
// MCP+LLM work.
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
	open, err := HasOpenWork(ctx, deps.Store, slug)
	if err != nil {
		return res, fmt.Errorf("check open work: %w", err)
	}
	if open {
		res.Skipped = true
		res.SkipReason = "persona already has open issues; not seeding to avoid duplicates"
		return res, nil
	}

	// Start a turn-event tracker. Helpers inside Draft (callAbility,
	// draftRewrite*) read it from context and append model + skill calls.
	// Recorder is nil-safe (tests pass nil; the persona still works), so
	// we always wrap the context — the cost is a context.WithValue alloc.
	tracker := telemetry.NewTracker(uuid.NewString(), slug)
	tctx := telemetry.WithTracker(ctx, tracker)

	d, err := p.Draft(tctx, deps)
	if err != nil {
		recordTurn(ctx, deps.Recorder, tracker, "", d)
		return res, err
	}
	if d.Skipped {
		recordTurn(ctx, deps.Recorder, tracker, "", d)
		res.Skipped = true
		res.SkipReason = d.SkipReason
		return res, nil
	}

	id, err := insertIssue(ctx, deps.Store, slug, d)
	if err != nil {
		recordTurn(ctx, deps.Recorder, tracker, "", d)
		return res, fmt.Errorf("insert issue: %w", err)
	}
	recordTurn(ctx, deps.Recorder, tracker, id, d)
	res.IssueID = id
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

	_, err := st.DB.ExecContext(ctx,
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, d.Title, d.Description, persona, "in_review", priority, now, now,
		d.ProposalType, d.ProposalContent, targetJSON,
	)
	if err != nil {
		return "", err
	}
	return id, nil
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
