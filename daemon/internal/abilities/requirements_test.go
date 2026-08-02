package abilities

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// schema0_1_0 is the real wooagent-products/list input schema as discovered
// from a store running Companion Plugin 0.1.0 — the one that produced
// DSGWOO-1471. Note the absence of orderby/order and
// additionalProperties:false, which together are why the call was rejected.
const schema0_1_0 = `{
  "description": "List WooCommerce products with optional filters.",
  "input_schema": {
    "additionalProperties": false,
    "properties": {
      "page":     {"type": "integer"},
      "per_page": {"type": "integer"},
      "search":   {"type": "string"},
      "status":   {"type": "string"}
    }
  }
}`

// schema0_4_0 is the same ability from a current plugin.
const schema0_4_0 = `{
  "input_schema": {
    "additionalProperties": false,
    "properties": {
      "page":     {"type": "integer"},
      "per_page": {"type": "integer"},
      "search":   {"type": "string"},
      "status":   {"type": "string"},
      "orderby":  {"type": "string"},
      "order":    {"type": "string"}
    }
  }
}`

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st.DB
}

func seedStore(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO stores(id, url, mcp_endpoint, status, created_at, updated_at)
		 VALUES(?, ?, ?, 'paired', '2026-08-02T00:00:00Z', '2026-08-02T00:00:00Z')`,
		id, "https://"+id+".example.com", "https://"+id+".example.com/wp-json/mcp/x")
	if err != nil {
		t.Fatalf("seed store %s: %v", id, err)
	}
}

func seedAbilitySchema(t *testing.T, db *sql.DB, storeID, name, schema string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO abilities (id, store_id, name, schema_json, schema_hash, trust_state, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'sha256:x', 'new', '2026-08-02T00:00:00Z', '2026-08-02T00:00:00Z', '2026-08-02T00:00:00Z')`,
		"ab_"+storeID+"_"+name, storeID, name, schema)
	if err != nil {
		t.Fatalf("seed ability %s: %v", name, err)
	}
}

// seedAllCurrent gives the store everything the daemon needs, so individual
// tests can remove exactly one thing.
func seedAllCurrent(t *testing.T, db *sql.DB, storeID string) {
	t.Helper()
	seedAbilitySchema(t, db, storeID, "wooagent-products/list", schema0_4_0)
	for _, n := range []string{
		"wooagent-products/get",
		"wooagent-products/variations-list",
		"wooagent-orders/list",
		"wooagent-orders/get",
	} {
		// Open schemas — these abilities take simple id/per_page inputs and
		// have not changed shape.
		seedAbilitySchema(t, db, storeID, n, `{"input_schema":{"properties":{"id":{},"product_id":{},"per_page":{}}}}`)
	}
}

func TestCheckStore_CurrentPluginHasNoGaps(t *testing.T) {
	db := openTestDB(t)
	seedStore(t, db, "store_ok")
	seedAllCurrent(t, db, "store_ok")

	gaps, err := CheckStore(context.Background(), db, "store_ok")
	if err != nil {
		t.Fatalf("CheckStore: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("expected no gaps, got %+v", gaps)
	}
}

// The DSGWOO-1471 store, reproduced: products/list exists but its 0.1.0
// schema rejects orderby/order, and variations-list isn't registered at all.
func TestCheckStore_DetectsOldPluginBeforeAnyRunHappens(t *testing.T) {
	db := openTestDB(t)
	seedStore(t, db, "store_old")
	seedAbilitySchema(t, db, "store_old", "wooagent-products/list", schema0_1_0)
	seedAbilitySchema(t, db, "store_old", "wooagent-products/get", `{"input_schema":{"properties":{"id":{}}}}`)
	seedAbilitySchema(t, db, "store_old", "wooagent-orders/list", `{"input_schema":{"properties":{"per_page":{}}}}`)
	seedAbilitySchema(t, db, "store_old", "wooagent-orders/get", `{"input_schema":{"properties":{"id":{}}}}`)
	// variations-list deliberately absent — 0.1.0 did not register it.

	gaps, err := CheckStore(context.Background(), db, "store_old")
	if err != nil {
		t.Fatalf("CheckStore: %v", err)
	}
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps, got %d: %+v", len(gaps), gaps)
	}

	byAbility := map[string]Gap{}
	for _, g := range gaps {
		byAbility[g.Ability] = g
	}

	list, ok := byAbility["wooagent-products/list"]
	if !ok {
		t.Fatal("expected a gap on wooagent-products/list")
	}
	if list.Missing {
		t.Error("products/list exists on this store; it should be a param gap, not missing")
	}
	if got := strings.Join(list.UnsupportedParams, ","); got != "order,orderby" {
		t.Errorf("UnsupportedParams = %q, want \"order,orderby\"", got)
	}
	if !strings.Contains(list.Summary(), "does not accept") {
		t.Errorf("Summary should name the rejected params: %q", list.Summary())
	}

	vl, ok := byAbility["wooagent-products/variations-list"]
	if !ok {
		t.Fatal("expected a gap on wooagent-products/variations-list")
	}
	if !vl.Missing {
		t.Error("variations-list is unregistered; Missing should be true")
	}
	if !strings.Contains(vl.Summary(), "not available") {
		t.Errorf("Summary should say it's unavailable: %q", vl.Summary())
	}
	// The message has to say who breaks, or the operator can't judge impact.
	if !strings.Contains(vl.Summary(), "Pricing") {
		t.Errorf("Summary should name the dependent persona: %q", vl.Summary())
	}
}

// An open schema accepts unknown keys, so an undeclared parameter is passed
// through rather than rejected. Flagging it would fail stores that work.
func TestCheckStore_OpenSchemaIsNotAGap(t *testing.T) {
	db := openTestDB(t)
	seedStore(t, db, "store_open")
	seedAllCurrent(t, db, "store_open")
	// Replace products/list with an open schema missing orderby entirely.
	if _, err := db.Exec(`UPDATE abilities SET schema_json=? WHERE store_id=? AND name=?`,
		`{"input_schema":{"properties":{"per_page":{}}}}`, "store_open", "wooagent-products/list"); err != nil {
		t.Fatalf("update: %v", err)
	}

	gaps, err := CheckStore(context.Background(), db, "store_open")
	if err != nil {
		t.Fatalf("CheckStore: %v", err)
	}
	for _, g := range gaps {
		if g.Ability == "wooagent-products/list" {
			t.Errorf("open schema should not be reported as a gap: %+v", g)
		}
	}
}

// Before discovery runs there is nothing to compare against. Reporting every
// requirement as missing would be noise on a store that is merely new.
func TestCheckStore_NoAbilitiesDiscoveredYet(t *testing.T) {
	db := openTestDB(t)
	seedStore(t, db, "store_fresh")

	gaps, err := CheckStore(context.Background(), db, "store_fresh")
	if err != nil {
		t.Fatalf("CheckStore: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("a store with no discovered abilities should report no gaps, got %+v", gaps)
	}
}

// A schema we can't parse is not evidence of a gap — guessing from a shape
// we don't understand would produce false alarms on unfamiliar stores.
func TestCheckStore_UnparseableSchemaIsNotAGap(t *testing.T) {
	db := openTestDB(t)
	seedStore(t, db, "store_weird")
	seedAllCurrent(t, db, "store_weird")
	if _, err := db.Exec(`UPDATE abilities SET schema_json='not json' WHERE store_id=? AND name=?`,
		"store_weird", "wooagent-products/list"); err != nil {
		t.Fatalf("update: %v", err)
	}

	gaps, err := CheckStore(context.Background(), db, "store_weird")
	if err != nil {
		t.Fatalf("CheckStore: %v", err)
	}
	for _, g := range gaps {
		if g.Ability == "wooagent-products/list" {
			t.Errorf("unparseable schema should not be reported as a gap: %+v", g)
		}
	}
}

// Gaps are scoped per store — one bad store must not taint another.
func TestCheckStore_IsPerStore(t *testing.T) {
	db := openTestDB(t)
	seedStore(t, db, "store_good")
	seedStore(t, db, "store_bad")
	seedAllCurrent(t, db, "store_good")
	seedAbilitySchema(t, db, "store_bad", "wooagent-products/list", schema0_1_0)

	good, err := CheckStore(context.Background(), db, "store_good")
	if err != nil {
		t.Fatalf("CheckStore(good): %v", err)
	}
	if len(good) != 0 {
		t.Errorf("healthy store reported gaps: %+v", good)
	}
	bad, err := CheckStore(context.Background(), db, "store_bad")
	if err != nil {
		t.Fatalf("CheckStore(bad): %v", err)
	}
	if len(bad) == 0 {
		t.Error("old-plugin store reported no gaps")
	}
}

// Guards the maintenance hazard called out on Requirements: a param added to
// a persona call but not declared here is silently unchecked.
func TestRequirements_AreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Requirements {
		if r.Ability == "" || len(r.Params) == 0 || r.UsedBy == "" {
			t.Errorf("incomplete requirement: %+v", r)
		}
		if seen[r.Ability] {
			t.Errorf("duplicate requirement for %s — merge the param lists", r.Ability)
		}
		seen[r.Ability] = true
	}
	// products/list is the one that broke; keep its params pinned.
	for _, r := range Requirements {
		if r.Ability != "wooagent-products/list" {
			continue
		}
		for _, want := range []string{"orderby", "order", "per_page", "status"} {
			if !contains(r.Params, want) {
				t.Errorf("products/list requirement missing %q", want)
			}
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
