package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// memSecrets is an in-process map masquerading as a secrets.Store. The
// keychain itself is integration-tested in internal/secrets; here we just
// need a deterministic Get for the cutover-merge tests.
type memSecrets map[string]string

func (m memSecrets) Set(_ context.Context, k, v string) error {
	m[k] = v
	return nil
}

func (m memSecrets) Get(_ context.Context, k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}

func (m memSecrets) Delete(_ context.Context, k string) error {
	if _, ok := m[k]; !ok {
		return secrets.ErrNotFound
	}
	delete(m, k)
	return nil
}

// openTestStore opens an in-memory SQLite store with all migrations applied,
// so resolve_test.go has a real model_providers table to read from.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// insertProvider inlines the SQL the handler uses; we don't go through the
// HTTP path here because resolve_test.go is testing the boot-time read, not
// the create handler.
func insertProvider(t *testing.T, st *store.Store, id, kind, endpoint, model, secretRef string, isDefault bool, createdAt string) {
	t.Helper()
	def := 0
	if isDefault {
		def = 1
	}
	_, err := st.DB.ExecContext(context.Background(), `
		INSERT INTO model_providers(id, kind, name, endpoint, default_model, secret_ref, is_default, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, kind, kind+" · "+model, nullable(endpoint), model, nullable(secretRef), def, createdAt, createdAt)
	if err != nil {
		t.Fatalf("insert provider: %v", err)
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// envOnlyBase mirrors what envFromOS() would produce on a host with both
// Anthropic and OpenAI keys set — gives us something to verify pass-through
// against.
func envOnlyBase() personas.Env {
	return personas.Env{
		AnthropicAPIKey: "env-anthropic-key",
		AnthropicModel:  "env-anthropic-model",
		OpenAIAPIKey:    "env-openai-key",
		OpenAIAPIBase:   "https://env.openai.example",
		OpenAIModel:     "env-openai-model",
	}
}

func TestResolvePersonaEnv_NoRow_PassesThroughEnv(t *testing.T) {
	st := openTestStore(t)
	var buf bytes.Buffer

	got := resolvePersonaEnv(context.Background(), st.DB, memSecrets{}, envOnlyBase(), &buf)

	if got != envOnlyBase() {
		t.Errorf("expected env to pass through unchanged, got %+v", got)
	}
	if !strings.Contains(buf.String(), "removed in v0.2") {
		t.Errorf("expected deprecation warning, got %q", buf.String())
	}
}

func TestResolvePersonaEnv_NoRow_NoEnv_StaysQuiet(t *testing.T) {
	st := openTestStore(t)
	var buf bytes.Buffer

	got := resolvePersonaEnv(context.Background(), st.DB, memSecrets{}, personas.Env{}, &buf)

	if got != (personas.Env{}) {
		t.Errorf("expected empty env, got %+v", got)
	}
	if buf.Len() != 0 {
		t.Errorf("expected silence when nothing is configured, got %q", buf.String())
	}
}

func TestResolvePersonaEnv_AnthropicDefault_OverridesAnthropicKeepsOpenAI(t *testing.T) {
	st := openTestStore(t)
	sec := memSecrets{"wooagent.model_providers.mp_a": "sqlite-anthropic-key"}
	insertProvider(t, st, "mp_a", "anthropic", "", "claude-sonnet-4-6",
		"wooagent.model_providers.mp_a", true, "2026-05-06T12:00:00Z")

	var buf bytes.Buffer
	got := resolvePersonaEnv(context.Background(), st.DB, sec, envOnlyBase(), &buf)

	if got.AnthropicAPIKey != "sqlite-anthropic-key" {
		t.Errorf("AnthropicAPIKey: want sqlite-anthropic-key, got %q", got.AnthropicAPIKey)
	}
	if got.AnthropicModel != "claude-sonnet-4-6" {
		t.Errorf("AnthropicModel: want claude-sonnet-4-6, got %q", got.AnthropicModel)
	}
	// OpenAI fields untouched — env-var fallback still feeds personas that use OpenAI.
	if got.OpenAIAPIKey != "env-openai-key" {
		t.Errorf("OpenAIAPIKey: want env-openai-key, got %q", got.OpenAIAPIKey)
	}
	if got.OpenAIAPIBase != "https://env.openai.example" {
		t.Errorf("OpenAIAPIBase: want env value, got %q", got.OpenAIAPIBase)
	}
	if !strings.Contains(buf.String(), "using anthropic") {
		t.Errorf("expected anthropic source log, got %q", buf.String())
	}
}

func TestResolvePersonaEnv_OpenAIWithEndpoint_OverridesBase(t *testing.T) {
	st := openTestStore(t)
	sec := memSecrets{"wooagent.model_providers.mp_o": "sqlite-openai-key"}
	insertProvider(t, st, "mp_o", "openai", "https://api.openai-proxy.example", "gpt-5.0",
		"wooagent.model_providers.mp_o", true, "2026-05-06T12:00:00Z")

	var buf bytes.Buffer
	got := resolvePersonaEnv(context.Background(), st.DB, sec, envOnlyBase(), &buf)

	if got.OpenAIAPIKey != "sqlite-openai-key" {
		t.Errorf("OpenAIAPIKey: want sqlite-openai-key, got %q", got.OpenAIAPIKey)
	}
	if got.OpenAIModel != "gpt-5.0" {
		t.Errorf("OpenAIModel: want gpt-5.0, got %q", got.OpenAIModel)
	}
	if got.OpenAIAPIBase != "https://api.openai-proxy.example" {
		t.Errorf("OpenAIAPIBase: want endpoint override, got %q", got.OpenAIAPIBase)
	}
	if got.AnthropicAPIKey != "env-anthropic-key" {
		t.Errorf("AnthropicAPIKey should pass through, got %q", got.AnthropicAPIKey)
	}
}

func TestResolvePersonaEnv_Ollama_PointsOpenAIBaseAtEndpoint(t *testing.T) {
	st := openTestStore(t)
	sec := memSecrets{}
	insertProvider(t, st, "mp_ll", "ollama", "http://localhost:11434", "llama3.2",
		"", true, "2026-05-06T12:00:00Z")

	var buf bytes.Buffer
	got := resolvePersonaEnv(context.Background(), st.DB, sec, envOnlyBase(), &buf)

	if got.OpenAIAPIBase != "http://localhost:11434" {
		t.Errorf("OpenAIAPIBase: want localhost:11434, got %q", got.OpenAIAPIBase)
	}
	if got.OpenAIModel != "llama3.2" {
		t.Errorf("OpenAIModel: want llama3.2, got %q", got.OpenAIModel)
	}
	// Ollama doesn't authenticate — env-var key should pass through unchanged.
	if got.OpenAIAPIKey != "env-openai-key" {
		t.Errorf("OpenAIAPIKey should pass through for Ollama, got %q", got.OpenAIAPIKey)
	}
}

func TestResolvePersonaEnv_SecretMissing_FallsBackToEnv(t *testing.T) {
	st := openTestStore(t)
	sec := memSecrets{} // empty — Get returns ErrNotFound
	insertProvider(t, st, "mp_a", "anthropic", "", "claude-sonnet-4-6",
		"wooagent.model_providers.mp_a", true, "2026-05-06T12:00:00Z")

	var buf bytes.Buffer
	got := resolvePersonaEnv(context.Background(), st.DB, sec, envOnlyBase(), &buf)

	if got != envOnlyBase() {
		t.Errorf("expected fallback to env, got %+v", got)
	}
	if !strings.Contains(buf.String(), "not in keychain") {
		t.Errorf("expected keychain-miss log, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "removed in v0.2") {
		t.Errorf("expected deprecation warning on env-var fallback, got %q", buf.String())
	}
}

func TestResolvePersonaEnv_NoIsDefault_PicksEarliestCreated(t *testing.T) {
	st := openTestStore(t)
	sec := memSecrets{
		"wooagent.model_providers.mp_first":  "first-key",
		"wooagent.model_providers.mp_second": "second-key",
	}
	// Neither row is_default. Older row should win.
	insertProvider(t, st, "mp_first", "anthropic", "", "claude-haiku-4-5",
		"wooagent.model_providers.mp_first", false, "2026-05-06T10:00:00Z")
	insertProvider(t, st, "mp_second", "anthropic", "", "claude-sonnet-4-6",
		"wooagent.model_providers.mp_second", false, "2026-05-06T12:00:00Z")

	var buf bytes.Buffer
	got := resolvePersonaEnv(context.Background(), st.DB, sec, envOnlyBase(), &buf)

	if got.AnthropicAPIKey != "first-key" {
		t.Errorf("expected earliest-created (first-key), got %q", got.AnthropicAPIKey)
	}
	if got.AnthropicModel != "claude-haiku-4-5" {
		t.Errorf("expected earliest-created model, got %q", got.AnthropicModel)
	}
}

func TestLoadDefaultModelProvider_EmptyTable(t *testing.T) {
	st := openTestStore(t)
	mp, err := loadDefaultModelProvider(context.Background(), st.DB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp != nil {
		t.Errorf("expected nil, got %+v", mp)
	}
}

func TestLoadDefaultModelProvider_PrefersIsDefault(t *testing.T) {
	st := openTestStore(t)
	// Older row, NOT default.
	insertProvider(t, st, "mp_old", "anthropic", "", "old-model", "ref-old", false, "2026-05-06T10:00:00Z")
	// Newer row IS default — should win even though older row sorts first by date.
	insertProvider(t, st, "mp_new", "openai", "", "new-model", "ref-new", true, "2026-05-06T12:00:00Z")

	mp, err := loadDefaultModelProvider(context.Background(), st.DB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp == nil || mp.ID != "mp_new" {
		t.Errorf("expected mp_new (is_default=1), got %+v", mp)
	}
}

func TestResolvePersonaEnv_UnknownKind_FallsBack(t *testing.T) {
	st := openTestStore(t)
	sec := memSecrets{"wooagent.model_providers.mp_x": "k"}
	// Bypass the validKinds check by inserting directly. Models a future
	// kind we haven't taught the resolver about yet.
	insertProvider(t, st, "mp_x", "future_kind", "", "some-model",
		"wooagent.model_providers.mp_x", true, "2026-05-06T12:00:00Z")

	var buf bytes.Buffer
	got := resolvePersonaEnv(context.Background(), st.DB, sec, envOnlyBase(), &buf)

	if got != envOnlyBase() {
		t.Errorf("expected env fallback for unknown kind, got %+v", got)
	}
	if !strings.Contains(buf.String(), "unknown kind") {
		t.Errorf("expected unknown-kind log, got %q", buf.String())
	}
}

// Sanity: secrets.ErrNotFound is the sentinel resolvePersonaEnv branches on.
// If the package ever renames it, this test catches the drift.
func TestSecretsErrNotFound_IsSentinel(t *testing.T) {
	if !errors.Is(secrets.ErrNotFound, secrets.ErrNotFound) {
		t.Fatal("ErrNotFound should be its own sentinel")
	}
}
