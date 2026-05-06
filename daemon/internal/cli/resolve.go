package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
)

// envFromOS reads the persona-relevant env vars into a personas.Env.
// Pulled out so resolvePersonaEnv can be unit-tested without process state.
func envFromOS() personas.Env {
	env := personas.Env{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  os.Getenv("ANTHROPIC_MODEL"),
		OpenAIAPIBase:   os.Getenv("OPENAI_API_BASE_URL"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:     os.Getenv("OPENAI_MODEL"),
		DefaultCurrency: os.Getenv("PERSONA_CURRENCY"),
	}
	if id := strings.TrimSpace(os.Getenv("PERSONA_PRODUCT_ID")); id != "" {
		if n, err := strconv.Atoi(id); err == nil {
			env.ProductIDOverride = n
		}
	}
	return env
}

// defaultModelProvider is the slice of a model_providers row that
// resolvePersonaEnv layers onto personas.Env.
type defaultModelProvider struct {
	ID           string
	Kind         string
	Endpoint     string
	DefaultModel string
	SecretRef    string
}

// loadDefaultModelProvider returns the row marked is_default=1, or the
// earliest-created row when none is marked. Returns (nil, nil) when the
// table is empty — the pre-onboarding state, handled by the caller as
// "stay on env vars".
func loadDefaultModelProvider(ctx context.Context, db *sql.DB) (*defaultModelProvider, error) {
	var mp defaultModelProvider
	var endpoint, secretRef sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT id, kind, endpoint, default_model, secret_ref
		FROM model_providers
		ORDER BY is_default DESC, datetime(created_at) ASC
		LIMIT 1
	`).Scan(&mp.ID, &mp.Kind, &endpoint, &mp.DefaultModel, &secretRef)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mp.Endpoint = endpoint.String
	mp.SecretRef = secretRef.String
	return &mp, nil
}

// resolvePersonaEnv returns the personas.Env the persona runner should use,
// layering the default model_providers row (if any) over the env-var-derived
// base. Logs go to out so the operator can see which source landed where.
//
// Layering rules:
//   - kind=anthropic → AnthropicAPIKey, AnthropicModel
//   - kind=openai    → OpenAIAPIKey, OpenAIModel; OpenAIAPIBase only when
//     the row has an endpoint set
//   - kind=ollama    → OpenAIAPIBase = endpoint, OpenAIModel = default_model.
//     Ollama is OpenAI-compatible at the persona's adapter layer; the row
//     carries no API key, so OpenAIAPIKey stays at whatever the env supplied.
//
// Errors loading the row or resolving its secret are non-fatal: log and fall
// back to env vars so a misconfigured row doesn't bench the whole fleet.
func resolvePersonaEnv(ctx context.Context, db *sql.DB, sec secrets.Store, base personas.Env, out io.Writer) personas.Env {
	mp, err := loadDefaultModelProvider(ctx, db)
	if err != nil {
		fmt.Fprintf(out, "→ model provider: lookup failed (%v); using env vars\n", err)
		return warnIfEnvOnly(base, out)
	}
	if mp == nil {
		return warnIfEnvOnly(base, out)
	}

	apiKey := ""
	if mp.SecretRef != "" {
		v, err := sec.Get(ctx, mp.SecretRef)
		switch {
		case errors.Is(err, secrets.ErrNotFound):
			fmt.Fprintf(out, "→ model provider %s (%s): secret_ref %q not in keychain; using env vars\n",
				mp.ID, mp.Kind, mp.SecretRef)
			return warnIfEnvOnly(base, out)
		case err != nil:
			fmt.Fprintf(out, "→ model provider %s (%s): keychain read failed (%v); using env vars\n",
				mp.ID, mp.Kind, err)
			return warnIfEnvOnly(base, out)
		}
		apiKey = v
	}

	env := base
	switch mp.Kind {
	case "anthropic":
		env.AnthropicAPIKey = apiKey
		env.AnthropicModel = mp.DefaultModel
	case "openai":
		env.OpenAIAPIKey = apiKey
		env.OpenAIModel = mp.DefaultModel
		if mp.Endpoint != "" {
			env.OpenAIAPIBase = mp.Endpoint
		}
	case "ollama":
		env.OpenAIAPIBase = mp.Endpoint
		env.OpenAIModel = mp.DefaultModel
	default:
		fmt.Fprintf(out, "→ model provider %s: unknown kind %q; using env vars\n", mp.ID, mp.Kind)
		return warnIfEnvOnly(base, out)
	}

	fmt.Fprintf(out, "→ model provider: using %s · %s from /v1/model-providers (id=%s)\n",
		mp.Kind, mp.DefaultModel, mp.ID)
	return env
}

// warnIfEnvOnly emits a deprecation notice when the persona runner is falling
// back to env vars while at least one provider env var is set. The env-var
// path stays functional in v0.1; v0.2 removes it.
func warnIfEnvOnly(env personas.Env, out io.Writer) personas.Env {
	if env.AnthropicAPIKey != "" || env.OpenAIAPIKey != "" || env.OpenAIAPIBase != "" {
		fmt.Fprintln(out, "→ model provider: using env vars (ANTHROPIC_API_KEY / OPENAI_*); these are removed in v0.2 — configure via Settings → Models or POST /v1/model-providers")
	}
	return env
}
