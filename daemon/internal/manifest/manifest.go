// Package manifest implements the pre-signed ability manifest from §8.4.1
// of the PRD. The manifest is the canonical allowlist: a discovered ability
// is trusted only if it matches an entry here by name, schema-hash, and
// version constraint — or if the operator has explicitly approved it at
// runtime (which extends the in-memory trust table but does not modify
// this shipped artifact).
//
// The default manifest is embedded in the daemon binary via //go:embed.
// Operators can layer a local override at ~/.wooagent/manifest.json; the
// loader merges operator entries on top of the shipped defaults.
package manifest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
)

// Scope describes the default authority level a pre-signed ability carries.
// Scopes compose with the PEP's per-invocation checks (persona permission,
// policy predicates, reversibility class); they are not a standalone gate.
type Scope string

const (
	// ScopeRead authorizes invocations that do not mutate store state.
	// Maps to read-only abilities: list, get, search.
	ScopeRead Scope = "read"

	// ScopePropose authorizes invocations that stage a change for review
	// without applying it. The change is visible in the In Review column
	// but has no live effect until the operator promotes it to apply.
	ScopePropose Scope = "propose"

	// ScopeApply authorizes invocations that take immediate effect on the
	// store. Reserved for abilities explicitly graduated to apply by the
	// operator, or for abilities that are inherently low-risk (e.g.,
	// creating a draft post, adding an internal order note).
	ScopeApply Scope = "apply"
)

// Persona is a slug identifying one of the v1 personas. Matches §7 of the PRD.
type Persona string

const (
	PersonaMarketing     Persona = "marketing"
	PersonaPricing       Persona = "pricing"
	PersonaInventory     Persona = "inventory"
	PersonaAccounting    Persona = "accounting"
	PersonaReporting     Persona = "reporting"
	PersonaSalesSupport  Persona = "sales-support"
	PersonaChiefOfStaff  Persona = "chief-of-staff"
)

// Entry is one pre-signed ability record. Every field is part of the trust
// decision except Description, which is informational only.
type Entry struct {
	// Ability is the fully-qualified ability name (e.g., "wooagent-products/list").
	// The namespace prefix is significant: the manifest trusts a specific
	// plugin's registrations, not any plugin that happens to register an
	// ability with a matching bare name.
	Ability string `json:"ability"`

	// NamespaceOwner identifies who is expected to register this namespace
	// (e.g., "wooagent-os/companion-plugin", "automattic/woocommerce"). This
	// is surfaced in the Ability explorer so the operator can verify
	// provenance at a glance.
	NamespaceOwner string `json:"namespace_owner"`

	// SourceURL points to the plugin's source of truth — repository, the
	// WordPress.org listing, or a vendor documentation page.
	SourceURL string `json:"source_url"`

	// PluginVersion is the plugin release this entry was computed against.
	// Used as a human-readable pin; drift detection relies on SchemaHash.
	PluginVersion string `json:"plugin_version,omitempty"`

	// SchemaHash is a canonical hash over the ability's input + output
	// schemas (see hash.go). A mismatch between the discovered ability's
	// computed hash and this value auto-demotes the ability to
	// `unapproved` and alerts the operator.
	SchemaHash string `json:"schema_hash"`

	// Scope is the default authority level (read / propose / apply).
	// Operators can relax scope per-ability in their local overlay.
	Scope Scope `json:"scope"`

	// Reversibility (0.0–1.0) feeds the approval-gate rules in §10.5:
	// a step whose primary ability has reversibility below a threshold
	// surfaces for per-step review even inside an approved plan.
	Reversibility float64 `json:"reversibility"`

	// Personas lists the persona slugs that are permitted to invoke this
	// ability by default. Empty list = ability is trusted but no agent
	// can invoke it (e.g., operator-facing bootstrapping abilities like
	// device-pair/*).
	Personas []Persona `json:"personas"`

	// Description is a one-line human-readable summary. Informational.
	Description string `json:"description,omitempty"`
}

// Manifest is the root document. Version is an integer we bump when the
// schema itself (not the entries) changes in a non-backward-compatible way.
type Manifest struct {
	Version     int     `json:"version"`
	GeneratedAt string  `json:"generated_at,omitempty"`
	Entries     []Entry `json:"entries"`
}

// TrustState is the per-ability classification the PEP reads when deciding
// whether to allow an invocation. See §8.4.1.
type TrustState string

const (
	TrustPreSigned         TrustState = "pre-signed"
	TrustOperatorApproved  TrustState = "operator-approved"
	TrustUnapproved        TrustState = "unapproved"
)

// Lookup indexes entries by ability name for O(1) access. It's a
// read-through helper; mutating it after construction is not supported.
type Lookup struct {
	byName map[string]*Entry
}

// NewLookup builds a Lookup from a Manifest. Duplicate ability names cause
// an error: the manifest should be unambiguous.
func NewLookup(m *Manifest) (*Lookup, error) {
	l := &Lookup{byName: make(map[string]*Entry, len(m.Entries))}
	for i := range m.Entries {
		e := &m.Entries[i]
		if _, dup := l.byName[e.Ability]; dup {
			return nil, fmt.Errorf("duplicate ability %q in manifest", e.Ability)
		}
		l.byName[e.Ability] = e
	}
	return l, nil
}

// Get returns the manifest entry for the given fully-qualified ability name,
// or nil if not pre-signed.
func (l *Lookup) Get(name string) *Entry {
	return l.byName[name]
}

// Size returns the number of entries.
func (l *Lookup) Size() int { return len(l.byName) }

//go:embed default.json
var defaultManifest []byte

// Default returns the manifest embedded at build time.
func Default() (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(defaultManifest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode default manifest: %w", err)
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if err := validateEntries(m.Entries); err != nil {
		return nil, err
	}
	return &m, nil
}

// Load reads a manifest file from path. Used for operator overlays.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := validateEntries(m.Entries); err != nil {
		return nil, err
	}
	return &m, nil
}

func validateEntries(entries []Entry) error {
	seen := make(map[string]struct{}, len(entries))
	for i, e := range entries {
		if e.Ability == "" {
			return fmt.Errorf("entry %d: ability is empty", i)
		}
		if _, dup := seen[e.Ability]; dup {
			return fmt.Errorf("duplicate ability %q", e.Ability)
		}
		seen[e.Ability] = struct{}{}
		if e.SchemaHash == "" {
			return fmt.Errorf("entry %q: schema_hash is empty", e.Ability)
		}
		switch e.Scope {
		case ScopeRead, ScopePropose, ScopeApply:
		default:
			return fmt.Errorf("entry %q: invalid scope %q", e.Ability, e.Scope)
		}
		if e.Reversibility < 0 || e.Reversibility > 1 {
			return fmt.Errorf("entry %q: reversibility %.2f out of [0,1]", e.Ability, e.Reversibility)
		}
	}
	return nil
}
