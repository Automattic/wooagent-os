package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelTester validates that a (kind, api_key, endpoint, default_model)
// combination can actually reach the upstream. The Server holds one for
// dependency-injection: production wires realModelTester (real HTTP calls);
// tests inject a fake so we don't burn API credits or depend on network.
type ModelTester interface {
	Test(ctx context.Context, req ModelTestArgs) ModelTestOutcome
}

type ModelTestArgs struct {
	Kind         string
	APIKey       string
	Endpoint     string
	DefaultModel string
}

// ModelTestOutcome is the structured result the test handler turns into JSON.
// OK=false carries Message; Models is best-effort (Ollama and OpenAI list,
// Anthropic doesn't, so we hard-code a known set there).
type ModelTestOutcome struct {
	OK      bool
	Message string
	Models  []string
}

// realModelTester runs the actual upstream calls. Each kind is one HTTP
// round-trip with a tight timeout — the UI is sitting on a "Test
// connection" spinner.
type realModelTester struct {
	client *http.Client
}

func newRealModelTester() *realModelTester {
	return &realModelTester{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *realModelTester) Test(ctx context.Context, a ModelTestArgs) ModelTestOutcome {
	switch a.Kind {
	case "anthropic":
		return t.testAnthropic(ctx, a)
	case "openai":
		return t.testOpenAI(ctx, a)
	case "ollama":
		return t.testOllama(ctx, a)
	default:
		return ModelTestOutcome{Message: "unsupported kind: " + a.Kind}
	}
}

// testAnthropic issues a 1-token /v1/messages call. Cheapest validation
// that the key is real and the model exists. Anthropic doesn't expose a
// /models endpoint, so we return a hard-coded list of currently-supported
// model IDs the UI can populate the "default model" dropdown with.
func (t *realModelTester) testAnthropic(ctx context.Context, a ModelTestArgs) ModelTestOutcome {
	model := a.DefaultModel
	if model == "" {
		model = "claude-haiku-4-5"
	}
	body := strings.NewReader(fmt.Sprintf(`{"model":%q,"max_tokens":1,"messages":[{"role":"user","content":"."}]}`, model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", body)
	if err != nil {
		return ModelTestOutcome{Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	res, err := t.client.Do(req)
	if err != nil {
		return ModelTestOutcome{Message: "could not reach api.anthropic.com: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ModelTestOutcome{Message: "Anthropic rejected the API key (401)"}
	}
	if res.StatusCode == http.StatusNotFound {
		return ModelTestOutcome{Message: "model not found: " + model}
	}
	if res.StatusCode/100 != 2 {
		return ModelTestOutcome{Message: fmt.Sprintf("Anthropic returned %d", res.StatusCode)}
	}
	return ModelTestOutcome{
		OK: true,
		Models: []string{
			"claude-opus-4-7",
			"claude-sonnet-4-6",
			"claude-haiku-4-5",
		},
	}
}

func (t *realModelTester) testOpenAI(ctx context.Context, a ModelTestArgs) ModelTestOutcome {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return ModelTestOutcome{Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+a.APIKey)

	res, err := t.client.Do(req)
	if err != nil {
		return ModelTestOutcome{Message: "could not reach api.openai.com: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ModelTestOutcome{Message: "OpenAI rejected the API key (401)"}
	}
	if res.StatusCode/100 != 2 {
		return ModelTestOutcome{Message: fmt.Sprintf("OpenAI returned %d", res.StatusCode)}
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &parsed)
	models := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		models = append(models, m.ID)
	}
	return ModelTestOutcome{OK: true, Models: models}
}

// testOllama probes /api/tags on the operator-supplied endpoint. No auth.
// Returns the installed model list — the UI's "click test to detect" flow
// uses this to populate the default-model dropdown.
func (t *realModelTester) testOllama(ctx context.Context, a ModelTestArgs) ModelTestOutcome {
	endpoint := strings.TrimRight(a.Endpoint, "/")
	if endpoint == "" {
		return ModelTestOutcome{Message: "endpoint is required for ollama"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/tags", nil)
	if err != nil {
		return ModelTestOutcome{Message: err.Error()}
	}
	res, err := t.client.Do(req)
	if err != nil {
		return ModelTestOutcome{Message: "could not reach Ollama at " + endpoint + ": " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return ModelTestOutcome{Message: fmt.Sprintf("Ollama returned %d", res.StatusCode)}
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &parsed)
	models := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		models = append(models, m.Name)
	}
	return ModelTestOutcome{OK: true, Models: models}
}
