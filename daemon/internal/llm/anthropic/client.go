package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultCallTimeout caps one single /v1/messages round-trip. The
// tool-use loop runs many of these back-to-back; the loop's outer ctx
// should carry the larger deadline.
const DefaultCallTimeout = 90 * time.Second

// DefaultMaxToolLoopIterations caps how many model → tool → model
// round-trips RunToolLoop will execute before bailing. The cap is a
// guardrail against prompts that get stuck calling tools without ever
// converging on a final text reply.
const DefaultMaxToolLoopIterations = 6

// ErrToolLoopExhausted is returned (wrapped) by RunToolLoop when the
// iteration cap is reached. The caller still gets back the partial
// trace and the most recent assistant content — partial is better than
// silent failure.
var ErrToolLoopExhausted = errors.New("anthropic: tool-use loop exhausted iteration cap")

// Client is a reusable wrapper around Anthropic's Messages API.
// Construct once per daemon process; the underlying http.Client is
// safe for concurrent use.
type Client struct {
	apiKey string
	model  string
	apiURL string
	http   *http.Client
}

// New returns a Client bound to one API key and one default model.
// The model can be overridden per-call via Request.Model.
func New(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		apiURL: APIURL,
		http:   &http.Client{Timeout: DefaultCallTimeout},
	}
}

// WithAPIURL overrides the endpoint URL. Tests use this to redirect
// calls to an httptest server; production code shouldn't need it.
func (c *Client) WithAPIURL(url string) *Client {
	c.apiURL = url
	return c
}

// WithHTTPClient overrides the underlying *http.Client (timeout,
// transport, etc.). Production code should not need this.
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	c.http = h
	return c
}

// Call sends one /v1/messages request and parses the response. It does
// NOT loop on tool_use blocks; for that, use RunToolLoop. Use Call
// directly when you don't need local tools (e.g. plain text or
// server-managed tools like web_search, where Anthropic handles the
// loop internally).
func (c *Client) Call(ctx context.Context, req Request) (*Response, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 2048
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build http request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", Version)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: http %d: %s", resp.StatusCode, string(raw))
	}

	var parsed Response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w; body=%s", err, string(raw))
	}
	if parsed.Error.Type != "" {
		return nil, fmt.Errorf("anthropic: api error: %s · %s", parsed.Error.Type, parsed.Error.Message)
	}
	return &parsed, nil
}

// LoopOpts configures RunToolLoop. Zero value uses defaults.
type LoopOpts struct {
	// MaxIterations caps tool round-trips. Defaults to
	// DefaultMaxToolLoopIterations if zero.
	MaxIterations int

	// OnToolStart fires for each tool_use block before the matching
	// handler runs. Use it to emit "Pricing is researching..."
	// progress events on long-running tools. May be nil — the loop
	// runs uninstrumented in that case.
	OnToolStart func(name string, input json.RawMessage)
}

// FinalMessage is the model's last assistant turn — the answer the
// caller surfaces to the user.
type FinalMessage struct {
	Content    []ContentBlock
	StopReason string
}

// Trace captures the full transcript of a RunToolLoop call for logging
// and debugging. Messages starts with the caller's input messages,
// then every assistant turn the model produced (text + tool_use) and
// every tool_result turn the loop synthesized in response.
type Trace struct {
	Messages   []Message
	Iterations int
	Usage      Usage
}

// RunToolLoop iterates Anthropic call → execute model's tool_use
// blocks via the registered handlers → feed tool_result back → repeat,
// until the model returns an assistant turn with no tool_use blocks
// (final answer) or the iteration cap is reached.
//
// On normal completion it returns (final, trace, nil). On iteration
// exhaustion it returns the last assistant content as a partial final
// message along with ErrToolLoopExhausted. The caller can still
// surface that text — partial is better than nothing.
//
// Tool handler failures (handler returned an error, or the model
// called a tool that isn't registered) are NOT loop failures: the loop
// sends the error back to the model as tool_result with is_error=true,
// and the model gets one more turn to recover or explain.
func (c *Client) RunToolLoop(
	ctx context.Context,
	req Request,
	handlers map[string]ToolHandler,
	opts LoopOpts,
) (FinalMessage, Trace, error) {
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = DefaultMaxToolLoopIterations
	}

	// Snapshot the caller's messages so we don't mutate the input slice.
	trace := Trace{Messages: append([]Message(nil), req.Messages...)}

	for i := 0; i < opts.MaxIterations; i++ {
		trace.Iterations = i + 1
		req.Messages = trace.Messages

		resp, err := c.Call(ctx, req)
		if err != nil {
			return FinalMessage{}, trace, err
		}
		trace.Usage.InputTokens += resp.Usage.InputTokens
		trace.Usage.OutputTokens += resp.Usage.OutputTokens

		// Record the assistant's turn.
		trace.Messages = append(trace.Messages, Message{
			Role:    "assistant",
			Content: resp.Content,
		})

		// Collect tool_use blocks. None → model is done.
		var toolUses []ContentBlock
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				toolUses = append(toolUses, block)
			}
		}
		if len(toolUses) == 0 {
			return FinalMessage{
				Content:    resp.Content,
				StopReason: resp.StopReason,
			}, trace, nil
		}

		// Execute each tool_use; gather tool_result blocks for the
		// next user turn.
		results := make([]ContentBlock, 0, len(toolUses))
		for _, use := range toolUses {
			if opts.OnToolStart != nil {
				opts.OnToolStart(use.Name, use.Input)
			}
			handler, ok := handlers[use.Name]
			if !ok {
				results = append(results, ToolResultBlock(
					use.ID,
					fmt.Sprintf("tool %q is not available", use.Name),
					true,
				))
				continue
			}
			out, err := handler.Execute(ctx, use.Input)
			if err != nil {
				results = append(results, ToolResultBlock(use.ID, err.Error(), true))
				continue
			}
			results = append(results, ToolResultBlock(use.ID, out, false))
		}

		trace.Messages = append(trace.Messages, Message{
			Role:    "user",
			Content: results,
		})
	}

	// Iteration cap hit. Return the most recent assistant turn as a
	// partial final.
	var partial FinalMessage
	for i := len(trace.Messages) - 1; i >= 0; i-- {
		if trace.Messages[i].Role == "assistant" {
			partial = FinalMessage{Content: trace.Messages[i].Content}
			break
		}
	}
	return partial, trace, fmt.Errorf("%w (cap=%d)", ErrToolLoopExhausted, opts.MaxIterations)
}
