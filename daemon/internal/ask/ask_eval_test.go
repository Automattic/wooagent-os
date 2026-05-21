//go:build eval

// Package ask_test (eval-only) — real-fixture eval harness.
//
// This test file is gated behind the `eval` build tag and the
// ANTHROPIC_API_KEY env var. It does NOT run as part of the default CI
// suite. To execute, set the env var and pass the tag:
//
//	ANTHROPIC_API_KEY=sk-... go test ./daemon/internal/ask -tags=eval -run TestAskEval -v
//
// Why opt-in:
//   - Each query is a real /v1/messages call against the live Anthropic
//     API. Cost ≈ tens of cents per full eval run; not free.
//   - Latency is human-scale (~5-60s per query); not snappy.
//   - The eval is non-deterministic — assertions are written for
//     observed-during-spike-hand-trace behavior and tolerate normal
//     LLM variance, but they're not bit-exact.
//
// What's covered today (initial landing for DSGWOO-1357):
//   - Eval harness: fixtures → seed → drive the CoS agent end-to-end
//     against the real LLM → record the tool-call trace → assert.
//   - ~6 representative CoS queries — one per assertion pattern, from
//     the spike's 22-query set. The remaining 16 land as the user
//     iterates.
//   - Specialist evals (Marketing / Pricing / Sales Support) are NOT
//     included yet: their toolbelts require an MCP layer
//     (list_products / get_product / list_orders / get_order) that
//     isn't faked here. Mock-MCP is a follow-up; see TODOs below.
//
// Threshold: CoS ≥ 5/6 in this initial cut (matching the spike's
// 20/22 floor scaled down). Below that means a real prompt regression
// — investigate before bumping the threshold up.

package ask_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
	"github.com/wooagent-os/wooagent-os/daemon/internal/ask/agents"
	"github.com/wooagent-os/wooagent-os/daemon/internal/ask/tools"
	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

const (
	// evalModel is the Anthropic model the eval drives. Match the
	// production askModel default so the eval predicts prod behavior.
	evalModel = "claude-sonnet-4-6"

	// passThreshold is the minimum number of queries that must pass
	// for the eval to be considered green. 5/6 = 83%, matches the
	// spike's 20/22 floor (90%) scaled to this smaller initial set.
	// Tighten as queries are added.
	passThreshold = 5
)

// TestAskEval is the top-level entry. Runs every registered CoS query
// against the real Anthropic API, records pass/fail, and asserts the
// total passes meet the threshold.
func TestAskEval(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping eval")
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	fx := canonicalFixtures(now)

	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	fx.seed(t, ctx, st.DB)

	t.Logf("eval fixtures: %s · %s", fx.summary(),
		"anchor now="+now.Format(time.RFC3339))

	client := anthropic.New(apiKey, evalModel)
	runner := &evalRunner{
		client: client,
		db:     st.DB,
		enq: func(_ context.Context, persona string) (scheduler.Run, error) {
			// Eval doesn't run a real scheduler; dispatch_persona
			// gets a synthetic run id so the assertion can verify
			// it propagated through the tool result.
			return scheduler.Run{ID: "rn_eval_" + persona, Persona: persona, Trigger: scheduler.TriggerOperatorAsked}, nil
		},
		limiter: tools.DefaultRateLimiter(),
	}

	var passed int
	for _, q := range cosEvalQueries() {
		t.Run(q.name, func(t *testing.T) {
			ok := runEvalQuery(t, ctx, runner, q)
			if ok {
				passed++
			}
		})
	}

	t.Logf("eval result: %d / %d passed (threshold: %d)",
		passed, len(cosEvalQueries()), passThreshold)
	if passed < passThreshold {
		t.Errorf("eval below threshold: %d / %d", passed, passThreshold)
	}
}

// evalRunner bundles the dependencies needed to invoke one chat turn
// against the real LLM. Kept stateful so the rate limiter doesn't
// reset between queries (operator dispatches across the eval should
// hit the same 2-per-60s gate they would in production).
type evalRunner struct {
	client  *anthropic.Client
	db      *sql.DB
	enq     tools.EnqueuerFunc
	limiter *tools.RateLimiter
}

// evalQuery is one row in the eval suite. Name is the t.Run subtest
// label. Setup runs before the query (e.g. seed extra state). Assert
// inspects the model's tool sequence + final text and returns nil on
// pass, an error explaining the failure otherwise.
type evalQuery struct {
	name   string
	user   string
	page   string
	assert func(t *testing.T, trace anthropic.Trace, finalText string) error
}

// runEvalQuery drives one query end-to-end. Builds the CoS toolbelt,
// runs the loop, prints the trace + assertion result, returns pass
// status.
func runEvalQuery(t *testing.T, ctx context.Context, r *evalRunner, q evalQuery) bool {
	t.Helper()

	toolHandlers := []anthropic.ToolHandler{
		&tools.ListProposalsTool{DB: r.db},
		&tools.GetProposalTool{DB: r.db},
		&tools.ListRunsTool{DB: r.db},
		&tools.GetRunTool{DB: r.db},
		&tools.ListAgentsTool{DB: r.db},
		&tools.DispatchTool{Enqueue: r.enq, Limiter: r.limiter},
	}

	system := agents.CoSPrompt("Linenly (eval)")
	user := q.user
	if q.page != "" {
		// Mirror the handler's page-context inlining so the model
		// behaves the same way it would in production.
		pc, _ := json.Marshal(map[string]any{"page": q.page, "visible_items": []any{}})
		user = user + "\n\n<page_context>\n" + string(pc) + "\n</page_context>"
	}

	final, trace, err := r.client.RunToolLoop(ctx,
		anthropic.Request{
			System: system,
			Messages: []anthropic.Message{
				anthropic.UserMessage(user),
			},
			Tools: anthropic.Definitions(toolHandlers...),
		},
		anthropic.Handlers(toolHandlers...),
		anthropic.LoopOpts{MaxIterations: 6},
	)
	if err != nil {
		t.Logf("[fail] %s — LLM call error: %v", q.name, err)
		return false
	}
	finalText := assistantTextFromContent(final.Content)

	t.Logf("[query] %s\n  user:   %s\n  iters:  %d\n  tools:  %s\n  reply:  %s",
		q.name, q.user, trace.Iterations,
		toolNamesFromTrace(trace), abbreviate(finalText, 200))

	if assertErr := q.assert(t, trace, finalText); assertErr != nil {
		t.Logf("[fail] %s — %v", q.name, assertErr)
		return false
	}
	t.Logf("[pass] %s", q.name)
	return true
}

// --------------------------------------------------------------- queries
//
// Each query mirrors one entry from `2026-05-20-chief-of-staff-prompt-
// eval-spike.md`. The numbering ("Q1", "Q3", etc.) refers to that
// document so it's easy to cross-reference what's covered.
//
// Assertion pattern legend:
//   - readChip:    tool call(s) + chip in final text
//   - dispatch:    dispatch_persona called + reply mentions persona + ETA
//   - declineRed:  decline + redirect to picker (no tool call attempted)
//   - clarify:     no tool call + asks a follow-up question
//   - rangeCheck:  reply contains an expected time-window phrase
//   - stuckCheck:  reply names the threshold + flags the right item

func cosEvalQueries() []evalQuery {
	return []evalQuery{
		{
			name: "Q1_what_needs_attention_first",
			user: "What needs my attention first?",
			page: "needs-review",
			assert: func(t *testing.T, trace anthropic.Trace, reply string) error {
				if !calledAny(trace, "list_proposals") {
					return fmt.Errorf("expected list_proposals call")
				}
				// "Linen Napkin" is the oldest pending (47 min); the
				// CoS rank-by-age heuristic should surface it.
				if !contains(reply, "Linen Napkin") {
					return fmt.Errorf("reply should name Linen Napkin (oldest pending); got %q", reply)
				}
				// Reference chip should be present.
				if !contains(reply, "i_mk_1") && !contains(reply, "proposal") {
					return fmt.Errorf("reply missing reference chip: %q", reply)
				}
				return nil
			},
		},
		{
			name: "Q2_dispatch_pricing",
			user: "Pricing, look at SKU-1234, competitor just dropped.",
			page: "needs-review",
			assert: func(t *testing.T, trace anthropic.Trace, reply string) error {
				if !calledAny(trace, "dispatch_persona") {
					return fmt.Errorf("expected dispatch_persona call")
				}
				if !containsAny(reply, "Pricing", "pricing") {
					return fmt.Errorf("reply should name Pricing; got %q", reply)
				}
				if !containsAny(reply, "minute", "min") {
					return fmt.Errorf("reply should mention ETA (minute); got %q", reply)
				}
				return nil
			},
		},
		{
			name: "Q3_whats_stuck",
			user: "What's stuck?",
			page: "needs-review",
			assert: func(t *testing.T, trace anthropic.Trace, reply string) error {
				if !calledAny(trace, "list_proposals") {
					return fmt.Errorf("expected list_proposals call")
				}
				// Cotton Pillow is the only fixture pending >24h.
				if !contains(reply, "Cotton Pillow") {
					return fmt.Errorf("reply should name Cotton Pillow (the >24h stuck item); got %q", reply)
				}
				// CoS prompt requires it to state the threshold.
				if !containsAny(reply, "24", "day") {
					return fmt.Errorf("reply should state the stuck threshold (24h/1d); got %q", reply)
				}
				return nil
			},
		},
		{
			name: "Q4_declined_out_of_scope",
			user: "What's the weather in San Francisco?",
			page: "needs-review",
			assert: func(t *testing.T, trace anthropic.Trace, reply string) error {
				// No tool calls expected for an out-of-scope question.
				if len(trace.Messages) > 2 {
					// Heuristic: an out-of-scope decline should be a
					// single assistant turn with no tool_use blocks.
					// Allow a tiny number for model variance.
					if toolCallCount(trace) > 0 {
						return fmt.Errorf("expected no tool calls for out-of-scope query; got %d", toolCallCount(trace))
					}
				}
				if !containsAny(reply, "can't", "cannot", "don't", "doesn't") {
					return fmt.Errorf("reply should decline cleanly; got %q", reply)
				}
				return nil
			},
		},
		{
			name: "Q5_what_did_agents_do_overnight",
			user: "What did the agents do overnight?",
			page: "board",
			assert: func(t *testing.T, trace anthropic.Trace, reply string) error {
				// Should call either list_runs or list_proposals with
				// a time window.
				if !calledAny(trace, "list_runs", "list_proposals") {
					return fmt.Errorf("expected list_runs or list_proposals")
				}
				// CoS prompt requires stating the time window for
				// relative-time queries — "overnight" maps to 21:00+
				// per the prompt. Accept any explicit window mention.
				if !containsAny(reply, "overnight", "since", "last night", "past", "9 pm", "21:00") {
					return fmt.Errorf("reply should name the time window; got %q", reply)
				}
				return nil
			},
		},
		{
			name: "Q6_unknown_dispatchable_persona",
			user: "Have Inventory check stock on SKU-1234.",
			page: "agents",
			assert: func(t *testing.T, trace anthropic.Trace, reply string) error {
				// The model may attempt the dispatch — the tool will
				// reject ("not a dispatchable specialist") and the
				// model recovers in the next turn. Either way the
				// final text should explain Inventory isn't available.
				if !containsAny(reply, "Inventory", "inventory") {
					return fmt.Errorf("reply should name Inventory; got %q", reply)
				}
				if !containsAny(reply, "not available", "not yet", "isn't available", "isn't live", "can't") {
					return fmt.Errorf("reply should explain Inventory isn't available; got %q", reply)
				}
				return nil
			},
		},
	}
}

// ------------------------------------------------------------ assertions

// calledAny returns true if the trace's tool_use blocks include any of
// the named tools.
func calledAny(trace anthropic.Trace, names ...string) bool {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	for _, m := range trace.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" && want[b.Name] {
				return true
			}
		}
	}
	return false
}

// toolCallCount counts the tool_use blocks across the entire trace.
func toolCallCount(trace anthropic.Trace) int {
	n := 0
	for _, m := range trace.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				n++
			}
		}
	}
	return n
}

// toolNamesFromTrace returns a flat list of tool names called in
// order, useful for the log header line.
func toolNamesFromTrace(trace anthropic.Trace) string {
	var names []string
	for _, m := range trace.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				names = append(names, b.Name)
			}
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, " → ")
}

// contains is strings.Contains (case-insensitive) — eval assertions
// shouldn't fail on capitalization differences.
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// containsAny returns true if the haystack contains any of the
// needles (case-insensitive).
func containsAny(s string, needles ...string) bool {
	low := strings.ToLower(s)
	for _, n := range needles {
		if strings.Contains(low, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// abbreviate truncates s to max chars and adds an ellipsis if cut.
// Keeps eval log lines readable.
func abbreviate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// assistantTextFromContent extracts plain text from the model's final
// content blocks. Mirrors the same-named helper in
// daemon/internal/httpapi/handlers_ask.go; duplicated here to keep
// the eval an at-arms-length consumer of the public package surface.
func assistantTextFromContent(blocks []anthropic.ContentBlock) string {
	var s string
	for _, b := range blocks {
		if b.Type == "text" {
			s += b.Text
		}
	}
	return s
}

// Keep the imported packages honest if the file is whittled down
// during edits.
var _ = ask.AgentChiefOfStaff
var _ = agents.CoSPrompt
