package store

import (
	"context"
	"database/sql"
)

// CurrentStoreID returns the id of the store the daemon is paired to, or ""
// when nothing is paired.
//
// Every issue insert stamps this so a proposal records which store it was
// made against (DSGWOO-1371). Without it, provenance has to be inferred from
// timestamps, and re-pairing leaves old proposals looking current — which is
// how the Ask Agent drawer ended up offering chips naming products from a
// store the daemon was no longer connected to.
//
// Resolves the same row as mcpresolve.pairedTarget and the suggestion floor:
// `status = 'paired' ORDER BY paired_at DESC LIMIT 1`. All three have to agree
// or a proposal gets stamped with one store while the MCP client writes to
// another. If you change the selection here, change it there.
//
// Errors and "nothing paired" both yield "". A stamped store id is useful
// metadata, not a precondition — failing an insert because the stores table
// was briefly unreadable would trade a small loss of provenance for a lost
// proposal, which is the worse outcome. Readers treat "" the same as a
// pre-migration row.
func CurrentStoreID(ctx context.Context, db *sql.DB) string {
	if db == nil {
		return ""
	}
	var id string
	err := db.QueryRowContext(ctx, `
		SELECT id
		FROM stores
		WHERE status = 'paired'
		ORDER BY paired_at DESC
		LIMIT 1
	`).Scan(&id)
	if err != nil {
		// sql.ErrNoRows is the ordinary "nothing paired yet" case.
		return ""
	}
	return id
}
