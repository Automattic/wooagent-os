package abilities

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Requirement is one ability the daemon depends on, together with the input
// parameters it may send.
//
// This exists because a store can register an ability whose schema is older
// than the daemon expects. The Companion Plugin gains parameters over time —
// `orderby` and `order` arrived on wooagent-products/list in plugin 0.2.0 —
// and its input schemas set additionalProperties:false, so a store running an
// older plugin rejects the call outright. The MCP adapter reports that as a
// bare "An error occurred while executing the tool", which tells an operator
// nothing (DSGWOO-1471).
//
// The daemon already discovers and stores every ability's input schema, so it
// can answer "will this call be rejected?" locally, before anyone runs a
// persona.
type Requirement struct {
	// Ability is the fully-qualified ability name.
	Ability string
	// Params are the input parameter names the daemon may send. Not every
	// call sends all of them; the union is what matters, since any one of
	// them being unsupported breaks some code path.
	Params []string
	// UsedBy names the personas that depend on it, so the operator-facing
	// message can say what will stop working.
	UsedBy string
}

// Requirements is what the daemon needs from a paired store.
//
// Keep this in step with the call sites. A parameter added to a persona's
// ability call without being added here simply isn't checked — the gap
// resurfaces as a runtime failure, which is the thing this exists to prevent.
var Requirements = []Requirement{
	{
		Ability: "wooagent-products/list",
		Params:  []string{"per_page", "status", "orderby", "order"},
		UsedBy:  "Marketing, Pricing",
	},
	{
		Ability: "wooagent-products/get",
		Params:  []string{"id"},
		UsedBy:  "Marketing, Pricing",
	},
	{
		Ability: "wooagent-products/variations-list",
		Params:  []string{"product_id"},
		UsedBy:  "Pricing",
	},
	{
		Ability: "wooagent-orders/list",
		Params:  []string{"per_page"},
		UsedBy:  "Sales Support",
	},
	{
		Ability: "wooagent-orders/get",
		Params:  []string{"id"},
		UsedBy:  "Sales Support",
	},
}

// Gap is one unmet requirement on a store.
type Gap struct {
	Ability string `json:"ability"`
	UsedBy  string `json:"used_by"`
	// Missing is true when the store doesn't register the ability at all.
	Missing bool `json:"missing,omitempty"`
	// UnsupportedParams are parameters the daemon sends that the store's
	// input schema rejects. Only populated when the ability exists.
	UnsupportedParams []string `json:"unsupported_params,omitempty"`
}

// Summary is a one-line operator-facing description of the gap.
func (g Gap) Summary() string {
	if g.Missing {
		return fmt.Sprintf("%s is not available on this store (needed by %s)", g.Ability, g.UsedBy)
	}
	return fmt.Sprintf("%s does not accept %s on this store (needed by %s)",
		g.Ability, strings.Join(g.UnsupportedParams, ", "), g.UsedBy)
}

// CheckStore compares Requirements against the abilities discovered for a
// store and returns everything that won't work. An empty slice means the
// store satisfies the daemon.
//
// Reads the schemas already captured by discovery rather than calling the
// store, so it's cheap enough to run on every discovery sweep.
func CheckStore(ctx context.Context, db *sql.DB, storeID string) ([]Gap, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name, COALESCE(schema_json, '') FROM abilities WHERE store_id = ?`, storeID)
	if err != nil {
		return nil, fmt.Errorf("load abilities: %w", err)
	}
	defer rows.Close()

	schemas := map[string]string{}
	for rows.Next() {
		var name, schema string
		if err := rows.Scan(&name, &schema); err != nil {
			return nil, fmt.Errorf("scan ability: %w", err)
		}
		schemas[name] = schema
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate abilities: %w", err)
	}

	// No abilities discovered yet is not a gap — discovery may not have run.
	// Reporting every requirement as missing would be noise, and wrong.
	if len(schemas) == 0 {
		return nil, nil
	}

	var gaps []Gap
	for _, req := range Requirements {
		schema, ok := schemas[req.Ability]
		if !ok {
			gaps = append(gaps, Gap{Ability: req.Ability, UsedBy: req.UsedBy, Missing: true})
			continue
		}
		if unsupported := unsupportedParams(schema, req.Params); len(unsupported) > 0 {
			gaps = append(gaps, Gap{
				Ability:           req.Ability,
				UsedBy:            req.UsedBy,
				UnsupportedParams: unsupported,
			})
		}
	}
	return gaps, nil
}

// abilitySchema is the slice of the stored get-ability-info payload that
// matters here.
type abilitySchema struct {
	InputSchema struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	} `json:"input_schema"`
}

// unsupportedParams returns the params the schema would reject, sorted.
//
// Only meaningful when the schema closes itself with
// additionalProperties:false — an open schema accepts unknown keys, so a
// parameter we send that it doesn't declare is passed through rather than
// rejected. Treating an open schema as a gap would flag stores that work.
//
// An unparseable or absent schema yields no gaps: discovery stores whatever
// the adapter returned, and guessing from a shape we don't understand would
// produce false alarms on exactly the stores we know least about.
func unsupportedParams(schemaJSON string, params []string) []string {
	if strings.TrimSpace(schemaJSON) == "" {
		return nil
	}
	var s abilitySchema
	if err := json.Unmarshal([]byte(schemaJSON), &s); err != nil {
		return nil
	}
	if s.InputSchema.AdditionalProperties == nil || *s.InputSchema.AdditionalProperties {
		return nil
	}
	if s.InputSchema.Properties == nil {
		return nil
	}
	var out []string
	for _, p := range params {
		if _, ok := s.InputSchema.Properties[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
