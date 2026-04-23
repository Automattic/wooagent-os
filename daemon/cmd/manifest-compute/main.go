// Command manifest-compute fetches abilities from a live WordPress store
// via the Abilities API, computes canonical schema hashes, and emits
// manifest entries with classification defaults pre-filled.
//
// Operators run this when seeding the manifest or when a plugin update
// changes schemas and we need fresh hashes. The output is human-reviewable
// before being committed — scope, persona mapping, reversibility, and
// description should all be examined.
//
// Run:
//
//	WOOAGENT_MCP_USER=... WOOAGENT_MCP_APP_PASSWORD=... \
//	go run ./cmd/manifest-compute \
//	  -store https://<your-store> \
//	  -filter 'wooagent-*,core/*'
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

type abilityDoc struct {
	Name         string          `json:"name"`
	Label        string          `json:"label"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Annotations  struct {
		// The original MCP annotation names (*.Hint) carry the real
		// signal on this adapter; the un-suffixed variants are left
		// null by the adapter's serializer.
		ReadOnlyHint    *bool `json:"readOnlyHint"`
		DestructiveHint *bool `json:"destructiveHint"`
		IdempotentHint  *bool `json:"idempotentHint"`
	} `json:"annotations,omitempty"`
}

type abilitiesResponse []abilityDoc

func main() {
	storeURL := flag.String("store", "", "base URL of the store (e.g. https://example.com)")
	filter := flag.String("filter", "", "comma-separated name prefixes to include (e.g. 'wooagent-*,core/*')")
	perPage := flag.Int("per-page", 100, "page size for the abilities listing")
	pretty := flag.Bool("pretty", true, "emit indented JSON")
	flag.Parse()

	if *storeURL == "" {
		log.Fatal("required: -store")
	}
	user := mustEnv("WOOAGENT_MCP_USER")
	pass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")

	prefixes := parseFilter(*filter)

	client := &http.Client{Timeout: 30 * time.Second}
	abilities, err := fetchAbilities(client, *storeURL, user, pass, *perPage)
	if err != nil {
		log.Fatalf("fetch abilities: %v", err)
	}

	entries := make([]manifest.Entry, 0, len(abilities))
	skipped := 0
	for _, a := range abilities {
		if !matchesAnyPrefix(a.Name, prefixes) {
			skipped++
			continue
		}
		hash, err := manifest.SchemaHash(a.InputSchema, a.OutputSchema)
		if err != nil {
			log.Fatalf("hash %s: %v", a.Name, err)
		}
		entries = append(entries, defaultEntryFor(a, hash))
	}

	m := manifest.Manifest{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:     entries,
	}

	enc := json.NewEncoder(os.Stdout)
	if *pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(m); err != nil {
		log.Fatalf("encode: %v", err)
	}
	log.Printf("emitted %d entries (%d abilities skipped by filter)", len(entries), skipped)
}

// defaultEntryFor produces a sensible starting Entry from an ability doc.
// Scope and reversibility are inferred from annotations + name verb; the
// operator reviews these before committing.
func defaultEntryFor(a abilityDoc, hash string) manifest.Entry {
	ns, action := splitAbility(a.Name)
	scope := inferScope(a, action)
	return manifest.Entry{
		Ability:        a.Name,
		NamespaceOwner: guessOwner(ns),
		SourceURL:      "",
		SchemaHash:     hash,
		Scope:          scope,
		Reversibility:  inferReversibility(a, scope, action),
		Personas:       defaultPersonasFor(ns, scope),
		Description:    strings.TrimSpace(a.Description),
	}
}

func inferScope(a abilityDoc, action string) manifest.Scope {
	readOnly := a.Annotations.ReadOnlyHint != nil && *a.Annotations.ReadOnlyHint
	destructive := a.Annotations.DestructiveHint != nil && *a.Annotations.DestructiveHint
	switch {
	case readOnly:
		return manifest.ScopeRead
	case destructive:
		return manifest.ScopeApply
	}
	// Fall back to name-based inference when annotations don't give a clear
	// signal. Verbs chosen from what actually appears in WP ability
	// registrations across the plugins surveyed.
	switch {
	case hasAnyPrefix(action, "list", "get", "search", "discover", "analyze", "read"):
		return manifest.ScopeRead
	case hasAnyPrefix(action, "delete", "revoke", "remove"):
		return manifest.ScopeApply
	default:
		return manifest.ScopePropose
	}
}

func inferReversibility(a abilityDoc, scope manifest.Scope, action string) float64 {
	if scope == manifest.ScopeRead {
		return 1.0
	}
	destructive := a.Annotations.DestructiveHint != nil && *a.Annotations.DestructiveHint
	switch {
	case destructive, hasAnyPrefix(action, "delete", "revoke", "remove"):
		return 0.2
	case hasAnyPrefix(action, "create"):
		return 0.7
	case hasAnyPrefix(action, "update", "add", "manage"):
		return 0.6
	default:
		return 0.5
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// defaultPersonasFor maps namespaces to personas. This is a starting point;
// operators tune via the local overlay.
func defaultPersonasFor(ns string, scope manifest.Scope) []manifest.Persona {
	all := []manifest.Persona{
		manifest.PersonaMarketing,
		manifest.PersonaPricing,
		manifest.PersonaInventory,
		manifest.PersonaAccounting,
		manifest.PersonaReporting,
		manifest.PersonaSalesSupport,
		manifest.PersonaChiefOfStaff,
	}
	switch ns {
	case "wooagent-products", "woocommerce":
		return []manifest.Persona{
			manifest.PersonaMarketing,
			manifest.PersonaPricing,
			manifest.PersonaInventory,
			manifest.PersonaSalesSupport,
		}
	case "wooagent-orders":
		return []manifest.Persona{
			manifest.PersonaAccounting,
			manifest.PersonaReporting,
			manifest.PersonaSalesSupport,
		}
	case "wooagent-customers":
		return []manifest.Persona{
			manifest.PersonaSalesSupport,
			manifest.PersonaMarketing,
		}
	case "wooagent-device-pair":
		// Operator-facing, never an agent path.
		return []manifest.Persona{}
	case "jetpack-forms":
		return []manifest.Persona{
			manifest.PersonaSalesSupport,
			manifest.PersonaMarketing,
		}
	case "core":
		return all
	case "yoast-seo":
		return []manifest.Persona{manifest.PersonaMarketing}
	case "acf":
		return []manifest.Persona{manifest.PersonaMarketing, manifest.PersonaInventory}
	default:
		return []manifest.Persona{manifest.PersonaChiefOfStaff}
	}
}

func guessOwner(ns string) string {
	switch ns {
	case "wooagent-products", "wooagent-orders", "wooagent-customers", "wooagent-device-pair":
		return "wooagent-os/companion-plugin"
	case "core":
		return "wordpress/wordpress"
	case "woocommerce":
		return "woocommerce/woocommerce"
	case "jetpack-forms":
		return "automattic/jetpack"
	case "yoast-seo":
		return "yoast/wordpress-seo"
	case "acf":
		return "wpengine/advanced-custom-fields"
	default:
		return ns
	}
}

func splitAbility(name string) (ns, action string) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return name, ""
	}
	return parts[0], parts[1]
}

func parseFilter(s string) []string {
	if s == "" {
		return nil
	}
	out := strings.Split(s, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func matchesAnyPrefix(name string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		// Treat "foo/*" and "foo-*" as prefix globs; anything else is an
		// exact match.
		if strings.HasSuffix(p, "/*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if name == p {
			return true
		}
	}
	return false
}

func fetchAbilities(client *http.Client, storeURL, user, pass string, perPage int) ([]abilityDoc, error) {
	all := []abilityDoc{}
	for page := 1; ; page++ {
		u := strings.TrimRight(storeURL, "/") + path.Join("/wp-json/wp-abilities/v1/abilities")
		u += fmt.Sprintf("?per_page=%d&page=%d", perPage, page)
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(user, pass)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("http %d page=%d body=%s", resp.StatusCode, page, string(body))
		}
		var pageAb abilitiesResponse
		if err := json.Unmarshal(body, &pageAb); err != nil {
			return nil, fmt.Errorf("decode page %d: %w  body=%s", page, err, string(body))
		}
		if len(pageAb) == 0 {
			break
		}
		all = append(all, pageAb...)
		if len(pageAb) < perPage {
			break
		}
	}
	return all, nil
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("required env var not set: %s", k)
	}
	return v
}
