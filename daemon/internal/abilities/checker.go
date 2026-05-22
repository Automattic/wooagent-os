package abilities

import (
	"context"
	"database/sql"
	"errors"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// Checker answers "is this ability invokable now?" for the connected
// store. Implements personas.Abilities by reading the trust state cached
// in the abilities table plus the bundled manifest pre-signs.
//
// The truth-table here mirrors pep.checkTrustState by design (DSGWOO-1361):
//
//	revoked_at set                                   -> false
//	manifest entry, placeholder SchemaHash           -> true   (WC 10.9 canonicals, trust by name)
//	manifest entry, no DB row yet                    -> false  (pre-discovery; transient)
//	manifest entry, DB hash matches manifest hash    -> true
//	manifest entry, DB hash differs from manifest    -> false  (schema drift)
//	abilities row exists, trust_state=trusted        -> true
//	anything else (no row, or row but
//	   trust_state in {new, schema_changed})         -> false
//
// If PEP's gate logic ever changes, this needs to track it — both should
// agree on what the persona is allowed to do, so the persona never picks
// an "enhanced" code path that will then get denied at invocation.
//
// Per-call DB read matches PEP's pattern (sub-ms SQLite locals). If a
// persona's Draft call invokes Has many times and perf shows up, snapshot
// once per Draft via the simple wrapper below.
type Checker struct {
	db       *sql.DB
	manifest *manifest.Lookup
}

// NewChecker builds an availability checker. Manifest may be nil — then
// only operator-trusted abilities ever return true.
func NewChecker(db *sql.DB, m *manifest.Lookup) *Checker {
	return &Checker{db: db, manifest: m}
}

// Has reports whether the named ability would currently pass PEP's
// trust gate. Returns false on any DB or lookup error — the conservative
// stance lets personas fall through to baseline paths instead of trying
// abilities that may not be reachable.
func (c *Checker) Has(name string) bool {
	if c == nil || c.db == nil {
		return false
	}
	var entry *manifest.Entry
	if c.manifest != nil {
		entry = c.manifest.Get(name)
	}

	var trustState string
	var revokedAt, schemaHash sql.NullString
	err := c.db.QueryRowContext(context.Background(),
		`SELECT trust_state, revoked_at, schema_hash FROM abilities WHERE name = ?`,
		name,
	).Scan(&trustState, &revokedAt, &schemaHash)
	if errors.Is(err, sql.ErrNoRows) {
		// Pre-discovery: only allow if the manifest entry is a placeholder
		// (WC 10.9 canonicals where we have no real hash to compare). Real-
		// hash manifest entries wait for the first discovery sweep.
		return entry != nil && entry.SchemaHash == manifest.PlaceholderSchemaHash
	}
	if err != nil {
		return false
	}
	if revokedAt.Valid && revokedAt.String != "" {
		return false
	}
	if entry != nil {
		if entry.SchemaHash == manifest.PlaceholderSchemaHash {
			return true
		}
		if !schemaHash.Valid || schemaHash.String == "" {
			return false
		}
		return schemaHash.String == entry.SchemaHash
	}
	return trustState == "trusted"
}
