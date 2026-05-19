package personas

import "testing"

// RegisterForTest registers p in the global persona registry and arranges
// for it to be removed when t finishes. Test-only: lets handler-level tests
// (e.g. /v1/agents in httpapi) populate the registry with deterministic
// fakes without dragging in the real personas/marketing etc. packages and
// their MCP/LLM dependencies.
//
// Calling Register directly from tests is also fine, but does not clean up
// — the global registry persists across tests in the same package and a
// later test that re-registers the same slug would panic.
func RegisterForTest(t testing.TB, p Persona) {
	t.Helper()
	Register(p)
	t.Cleanup(func() {
		regMu.Lock()
		defer regMu.Unlock()
		delete(reg, p.Slug())
	})
}
