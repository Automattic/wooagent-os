// Package abilities owns the daemon-side ability cache: discovery against
// paired stores via the WP MCP Adapter, hashing schemas to detect drift,
// and the trust state machine (new → trusted → schema_changed → trusted).
//
// Triggers (wired by the daemon main):
//   - on store create: kicked from the pairing approval path so a freshly
//     paired store has its abilities populated before the operator looks
//     at the Abilities screen.
//   - on daemon startup: a one-shot sweep over every paired store catches
//     drift accumulated while the daemon was off.
//   - periodic: a ticker re-runs RunAll every PollInterval to catch changes
//     without operator action.
//
// Trust state:
//   new             — first sighting, no operator approval recorded.
//   trusted         — operator approved at trusted_hash; matches current
//                     schema_hash.
//   schema_changed  — operator-approved row whose schema_hash diverges
//                     from trusted_hash. Trust expires automatically;
//                     re-approval bumps trusted_hash.
//
// The MCP client is created per discovery (it owns a session id), not held
// long-term, because sessions are per-store and not shared.
package abilities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
)

// MCPClientFactory builds an mcp.Client for a given store. Real production
// uses NewMCPClient; tests inject a fake that returns a stub client wired
// to an httptest server. The factory pattern keeps secrets handling and
// HTTP-transport details out of the runner's hot path.
type MCPClientFactory func(ctx context.Context, endpoint, bearerToken string) (Client, error)

// Client is the slice of mcp.Client the runner needs. Pulled out to an
// interface so tests don't need a real JSON-RPC server.
type Client interface {
	Initialize(ctx context.Context) (mcp.ServerInfo, error)
	DiscoverAbilities(ctx context.Context) ([]mcp.AbilitySummary, error)
	GetAbilityInfo(ctx context.Context, name string) (mcp.AbilityInfo, error)
}

// NewMCPClient is the production MCPClientFactory.
func NewMCPClient(ctx context.Context, endpoint, bearerToken string) (Client, error) {
	c := mcp.NewClient(mcp.Config{Endpoint: endpoint, BearerToken: bearerToken})
	if _, err := c.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcp initialize %s: %w", endpoint, err)
	}
	return c, nil
}

// Runner discovers abilities for paired stores and reconciles the
// abilities table. Safe for concurrent RunForStore calls — each call gets
// its own MCP client + DB transaction.
//
// Manifest is the pre-signed allowlist. It's consulted by SeedManifest /
// SeedAll to pre-populate the abilities table at startup (DSGWOO-1361
// cold-start UX fix), so the PEP's schema-hash gate can pass on the very
// first invocation without waiting for the async discovery sweep to
// complete. May be nil in tests that don't exercise seeding.
type Runner struct {
	DB           *sql.DB
	Secrets      secrets.Store
	Manifest     *manifest.Manifest
	NewClient    MCPClientFactory
	Logger       *slog.Logger
	PollInterval time.Duration // 0 means "no periodic poll"
}

// New returns a Runner with sensible defaults. Pass nil for m if the
// caller doesn't need cold-start seeding (e.g., unit tests that only
// exercise discovery).
func New(db *sql.DB, sec secrets.Store, m *manifest.Manifest) *Runner {
	return &Runner{
		DB:           db,
		Secrets:      sec,
		Manifest:     m,
		NewClient:    NewMCPClient,
		Logger:       slog.Default(),
		PollInterval: 6 * time.Hour,
	}
}

// RunForStore discovers abilities for one paired store and reconciles the
// cache. Returns the count of abilities cached after reconciliation. A
// store that's not in 'paired' status is a no-op (returns 0, nil) so the
// pairing approval path can fire-and-forget without an extra status check.
func (r *Runner) RunForStore(ctx context.Context, storeID string) (int, error) {
	var endpoint, status string
	var tokenRef sql.NullString
	err := r.DB.QueryRowContext(ctx,
		`SELECT mcp_endpoint, status, token_ref FROM stores WHERE id = ?`, storeID,
	).Scan(&endpoint, &status, &tokenRef)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store %s not found", storeID)
	}
	if err != nil {
		return 0, fmt.Errorf("load store %s: %w", storeID, err)
	}
	if status != "paired" {
		r.Logger.Debug("abilities: skip non-paired store", "store_id", storeID, "status", status)
		return 0, nil
	}
	if !tokenRef.Valid || tokenRef.String == "" {
		return 0, fmt.Errorf("store %s paired but has no token_ref", storeID)
	}

	token, err := r.Secrets.Get(ctx, tokenRef.String)
	if err != nil {
		return 0, fmt.Errorf("read device token for %s: %w", storeID, err)
	}

	client, err := r.NewClient(ctx, endpoint, token)
	if err != nil {
		return 0, err
	}

	summaries, err := client.DiscoverAbilities(ctx)
	if err != nil {
		return 0, fmt.Errorf("discover %s: %w", storeID, err)
	}

	// Pull full info per ability. We fetch sequentially to keep load on
	// the WP host predictable; at expected v0.1 cardinalities (dozens to
	// low hundreds) this completes in a few seconds. Fan-out can land
	// later if it becomes a UX bottleneck.
	infos := make([]mcp.AbilityInfo, 0, len(summaries))
	for _, sum := range summaries {
		info, err := client.GetAbilityInfo(ctx, sum.Name)
		if err != nil {
			r.Logger.Warn("abilities: get-ability-info failed; using summary only",
				"store_id", storeID, "name", sum.Name, "err", err)
			info = mcp.AbilityInfo{
				Name:        sum.Name,
				Title:       sum.Title,
				Description: sum.Description,
				Version:     sum.Version,
			}
		}
		infos = append(infos, info)
	}

	count, err := r.reconcile(ctx, storeID, infos)
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.DB.ExecContext(ctx,
		`UPDATE stores SET last_discovered_at=?, ability_count=?, updated_at=? WHERE id=?`,
		now, count, now, storeID,
	); err != nil {
		return count, fmt.Errorf("update stores summary for %s: %w", storeID, err)
	}
	r.Logger.Info("abilities: discovery complete", "store_id", storeID, "count", count)
	return count, nil
}

// reconcile inserts/updates abilities rows for the given store and
// deletes rows whose names no longer appear upstream. Trust state
// transitions per the package doc.
func (r *Runner) reconcile(ctx context.Context, storeID string, infos []mcp.AbilityInfo) (int, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	type existing struct {
		id          string
		schemaHash  string
		trustedHash sql.NullString
		trustState  string
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, schema_hash, trusted_hash, trust_state FROM abilities WHERE store_id = ?`, storeID)
	if err != nil {
		return 0, err
	}
	prior := map[string]existing{}
	for rows.Next() {
		var name string
		var ex existing
		if err := rows.Scan(&ex.id, &name, &ex.schemaHash, &ex.trustedHash, &ex.trustState); err != nil {
			_ = rows.Close()
			return 0, err
		}
		prior[name] = ex
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	_ = rows.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	seen := make(map[string]struct{}, len(infos))

	for _, info := range infos {
		seen[info.Name] = struct{}{}
		schemaJSON, schemaHash, err := canonicalize(info)
		if err != nil {
			return 0, fmt.Errorf("hash %s: %w", info.Name, err)
		}

		ex, exists := prior[info.Name]
		if !exists {
			id := "ab_" + uuid.NewString()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO abilities(id, store_id, name, title, description, version,
				                       schema_json, schema_hash, trust_state,
				                       last_seen_at, created_at, updated_at)
				 VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'new', ?, ?, ?)`,
				id, storeID, info.Name, info.Title, info.Description, info.Version,
				schemaJSON, schemaHash, now, now, now,
			); err != nil {
				return 0, fmt.Errorf("insert ability %s: %w", info.Name, err)
			}
			continue
		}

		newState := ex.trustState
		switch {
		case !ex.trustedHash.Valid || ex.trustedHash.String == "":
			// Never trusted — stays 'new' regardless of hash drift; the
			// operator will see whatever schema is current when they
			// approve.
			newState = "new"
		case schemaHash == ex.trustedHash.String:
			// Hash matches the trusted snapshot — return to 'trusted'
			// even if it had been 'schema_changed' previously (the
			// upstream rolled back).
			newState = "trusted"
		default:
			newState = "schema_changed"
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE abilities SET title=?, description=?, version=?,
			                      schema_json=?, schema_hash=?, trust_state=?,
			                      last_seen_at=?, updated_at=?
			 WHERE id=?`,
			info.Title, info.Description, info.Version,
			schemaJSON, schemaHash, newState, now, now, ex.id,
		); err != nil {
			return 0, fmt.Errorf("update ability %s: %w", info.Name, err)
		}
	}

	// Remove rows whose names are no longer reported upstream. Re-pairing
	// or re-installing the ability will resurrect a row in 'new' state —
	// trust is intentionally not preserved across removals.
	for name, ex := range prior {
		if _, kept := seen[name]; kept {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM abilities WHERE id=?`, ex.id,
		); err != nil {
			return 0, fmt.Errorf("delete stale ability %s: %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit reconcile %s: %w", storeID, err)
	}
	return len(infos), nil
}

// canonicalize returns (envelope-json, schema-hash) for an AbilityInfo.
//
// envelope-json carries the full info blob (name + metadata + schemas +
// permissions + required_scopes) and is stored in abilities.schema_json
// so checkSchema can later extract input_schema for arg validation.
//
// schema-hash is the canonical hash of the input + output schemas only —
// computed via manifest.SchemaHash so it matches the hash format the
// manifest carries on the other side of the trust gate (DSGWOO-1361).
// Trust drift is about schema-contract drift, not metadata drift: a
// description tweak or version bump should not invalidate operator
// approval, and the manifest's SchemaHash deliberately excludes those.
//
// Permissions and required_scopes participate in the envelope (so
// checkSchema sees them when needed) but not in the hash — the manifest
// doesn't carry them either, so the gate would have nothing to compare
// against.
func canonicalize(info mcp.AbilityInfo) (string, string, error) {
	body := map[string]any{
		"name":        info.Name,
		"title":       info.Title,
		"description": info.Description,
		"version":     info.Version,
	}
	if len(info.InputSchema) > 0 {
		var v any
		if err := json.Unmarshal(info.InputSchema, &v); err != nil {
			return "", "", fmt.Errorf("input_schema: %w", err)
		}
		body["input_schema"] = v
	}
	if len(info.OutputSchema) > 0 {
		var v any
		if err := json.Unmarshal(info.OutputSchema, &v); err != nil {
			return "", "", fmt.Errorf("output_schema: %w", err)
		}
		body["output_schema"] = v
	}
	if len(info.Permissions) > 0 {
		perms := append([]string{}, info.Permissions...)
		sort.Strings(perms)
		body["permissions"] = perms
	}
	if len(info.RequiredScopes) > 0 {
		scopes := append([]string{}, info.RequiredScopes...)
		sort.Strings(scopes)
		body["required_scopes"] = scopes
	}
	out, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	hash, err := manifest.SchemaHash(info.InputSchema, info.OutputSchema)
	if err != nil {
		return "", "", fmt.Errorf("schema hash: %w", err)
	}
	return string(out), hash, nil
}

// SeedManifest pre-populates the abilities table with rows for every
// manifest entry that doesn't already have one for storeID. Without this,
// the PEP's schema-hash trust gate (DSGWOO-1361) would deny manifest-
// pre-signed invocations with ReasonAbilityNotYetDiscovered between
// daemon-start and first-sweep-completion — a window that can stretch to
// seconds on a slow store.
//
// The seeded schema_hash is the manifest's SchemaHash, so the gate's
// "discovered hash matches manifest hash" branch passes immediately.
// When the async discovery sweep later runs against the live store,
// reconcile UPDATEs the row's schema_hash to the live value: if the
// live store matches the manifest, the row is unchanged and the gate
// keeps passing; if it differs, the row's schema_hash diverges from
// entry.SchemaHash and the next gate call returns ReasonSchemaDrift.
// Drift detection still works — we just trust the manifest's claim
// while discovery is in flight.
//
// Idempotent: existing rows are never overwritten (relies on the
// (store_id, name) UNIQUE constraint; the SELECT skips them). Safe to
// call on every daemon start.
//
// Returns the number of rows inserted.
func (r *Runner) SeedManifest(ctx context.Context, storeID string) (int, error) {
	if r.Manifest == nil || len(r.Manifest.Entries) == 0 {
		return 0, nil
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT name FROM abilities WHERE store_id = ?`, storeID)
	if err != nil {
		return 0, fmt.Errorf("list existing abilities %s: %w", storeID, err)
	}
	existing := map[string]struct{}{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return 0, err
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	_ = rows.Close()

	// last_seen_at carries "now" at seed time even though no live
	// discovery has touched the row yet — the column is NOT NULL and
	// reconcile will overwrite it on the first sweep moments later.
	now := time.Now().UTC().Format(time.RFC3339)
	inserted := 0
	for i := range r.Manifest.Entries {
		e := &r.Manifest.Entries[i]
		if _, ok := existing[e.Ability]; ok {
			continue
		}
		id := "ab_" + uuid.NewString()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO abilities(id, store_id, name, description, version,
			                       schema_hash, trust_state,
			                       last_seen_at, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, 'new', ?, ?, ?)`,
			id, storeID, e.Ability, e.Description, e.PluginVersion,
			e.SchemaHash, now, now, now,
		); err != nil {
			return inserted, fmt.Errorf("seed ability %s: %w", e.Ability, err)
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return inserted, fmt.Errorf("commit seed %s: %w", storeID, err)
	}
	if inserted > 0 {
		r.Logger.Info("abilities: manifest seed complete",
			"store_id", storeID, "inserted", inserted)
	}
	return inserted, nil
}

// SeedAll seeds manifest rows for every paired store. Intended for
// synchronous use at daemon startup, before the HTTP server starts
// accepting requests, so personas can invoke manifest-pre-signed
// abilities immediately. Errors are logged per-store and don't abort
// the loop — one broken pairing shouldn't bench the rest.
func (r *Runner) SeedAll(ctx context.Context) {
	if r.Manifest == nil || len(r.Manifest.Entries) == 0 {
		return
	}
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id FROM stores WHERE status = 'paired'`)
	if err != nil {
		r.Logger.Error("abilities: list paired stores for seed", "err", err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			r.Logger.Error("abilities: scan store id for seed", "err", err)
			return
		}
		ids = append(ids, id)
	}
	_ = rows.Close()

	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if _, err := r.SeedManifest(ctx, id); err != nil {
			r.Logger.Warn("abilities: seed failed", "store_id", id, "err", err)
		}
	}
}

// RunAll sweeps every paired store. Errors are logged per-store and don't
// abort the sweep — one broken pairing shouldn't stop the others from
// refreshing.
func (r *Runner) RunAll(ctx context.Context) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id FROM stores WHERE status = 'paired'`)
	if err != nil {
		r.Logger.Error("abilities: list paired stores", "err", err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			r.Logger.Error("abilities: scan store id", "err", err)
			return
		}
		ids = append(ids, id)
	}
	_ = rows.Close()

	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if _, err := r.RunForStore(ctx, id); err != nil {
			r.Logger.Warn("abilities: discovery failed", "store_id", id, "err", err)
		}
	}
}

// SchedulePeriodic launches a goroutine that calls RunAll every
// PollInterval until ctx is cancelled. No-op if PollInterval is 0.
func (r *Runner) SchedulePeriodic(ctx context.Context) {
	if r.PollInterval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(r.PollInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.RunAll(ctx)
			}
		}
	}()
}
