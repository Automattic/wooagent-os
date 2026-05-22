package ask

import (
	"encoding/json"
	"log/slog"
	"regexp"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
)

// proposalRefRE matches [proposal #N] / [proposal N] / [Proposal #N]
// in the model's prose. The id capture allows alphanumeric + dash /
// underscore to keep up with future id schemes.
var proposalRefRE = regexp.MustCompile(`\[[Pp]roposal\s+#?([A-Za-z0-9_-]+)\]`)

// runRefRE matches [run rn_X] / [run X] / [Run rn_X].
var runRefRE = regexp.MustCompile(`\[[Rr]un\s+#?([A-Za-z0-9_-]+)\]`)

// ExtractReferencesAndDispatched walks an Anthropic tool-use trace,
// indexes every proposal/run record the model saw in tool_result
// blocks (plus anything carried in the operator's PageContext), then
// matches them against the references the model actually cited in the
// final assistant text.
//
// IDs the model invents — present in the prose but never in any tool
// result or page-context item — are dropped (DSGWOO-1362). The chip
// row is a navigation affordance; a chip that 404s reads as broken,
// so the validation gate trades "trust the prose" for "trust only
// what the tools / page context confirm." Each drop is logged at
// Warn so we keep visibility on hallucination rate per agent.
//
// Returned references appear in citation order (first mention wins);
// duplicates within the text are deduped on id. Dispatched receipts
// come from every successful dispatch_persona call across the trace,
// regardless of whether the model mentioned them.
func ExtractReferencesAndDispatched(
	finalText string,
	trace anthropic.Trace,
	pageCtx *PageContext,
	agent AgentSlug,
) ([]Reference, []Dispatched) {
	proposalsByID, runsByID, dispatchedList := scanTrace(trace)
	absorbPageContext(pageCtx, proposalsByID, runsByID)

	var refs []Reference
	seen := map[string]bool{}

	// Proposals — iterate in order of mention so the chip list mirrors
	// the prose.
	for _, m := range proposalRefRE.FindAllStringSubmatch(finalText, -1) {
		id := m[1]
		key := "proposal:" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		rec, ok := proposalsByID[id]
		if !ok {
			slog.Warn("ask: dropping hallucinated proposal reference",
				"agent", agent, "id", id)
			continue
		}
		refs = append(refs, Reference{
			Kind:  "proposal",
			ID:    id,
			Title: rec.title,
			State: rec.state,
		})
	}
	for _, m := range runRefRE.FindAllStringSubmatch(finalText, -1) {
		id := m[1]
		key := "run:" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		rec, ok := runsByID[id]
		if !ok {
			slog.Warn("ask: dropping hallucinated run reference",
				"agent", agent, "id", id)
			continue
		}
		refs = append(refs, Reference{
			Kind:  "run",
			ID:    id,
			Title: rec.title,
			State: rec.state,
		})
	}

	return refs, dispatchedList
}

type seenRecord struct {
	title string
	state string
}

// scanTrace pulls every recognizable proposal / run / dispatch record
// out of the trace's tool_result blocks. Tool errors are skipped —
// the model already gets the error inline and won't cite a record it
// didn't successfully fetch.
//
// Recognized tool-output shapes (matches `daemon/internal/ask/tools`):
//
//   - list_proposals: { proposals: [{id, title, state, ...}], ... }
//   - get_proposal:   { id, title, state, ... }
//   - list_runs:      { runs: [{id, persona, status, ...}], ... }
//   - get_run:        { id, persona, status, ... }
//   - dispatch_persona (success): { ok: true, persona, run_id,
//     eta_seconds, target, brief }
//
// Unrecognized shapes are silently ignored; tool authors keep the
// freedom to add new tools without breaking this scanner.
func scanTrace(trace anthropic.Trace) (
	proposalsByID map[string]seenRecord,
	runsByID map[string]seenRecord,
	dispatched []Dispatched,
) {
	proposalsByID = map[string]seenRecord{}
	runsByID = map[string]seenRecord{}

	for _, msg := range trace.Messages {
		for _, block := range msg.Content {
			if block.Type != "tool_result" || block.IsError || block.ToolResultContent == "" {
				continue
			}
			absorbToolResult(block.ToolResultContent, proposalsByID, runsByID, &dispatched)
		}
	}
	return
}

// absorbPageContext indexes any proposal/run IDs the operator's
// current view exposes. The model receives the same page_context as a
// fenced JSON block on the user turn (see handlers_ask.go's
// buildAnthropicMessages), so it can legitimately cite those IDs
// without tool-calling — most commonly when the drawer opens on a
// board page and the operator asks about something on screen.
func absorbPageContext(
	pageCtx *PageContext,
	proposals map[string]seenRecord,
	runs map[string]seenRecord,
) {
	if pageCtx == nil {
		return
	}
	for _, item := range pageCtx.VisibleItems {
		if item.ID == "" {
			continue
		}
		rec := seenRecord{title: item.Title, state: item.State}
		switch item.Kind {
		case "proposal":
			if _, already := proposals[item.ID]; !already {
				proposals[item.ID] = rec
			}
		case "run":
			if _, already := runs[item.ID]; !already {
				runs[item.ID] = rec
			}
		}
	}
}

func absorbToolResult(
	raw string,
	proposals map[string]seenRecord,
	runs map[string]seenRecord,
	dispatched *[]Dispatched,
) {
	// list_proposals
	var lp struct {
		Proposals []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			State string `json:"state"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(raw), &lp); err == nil && len(lp.Proposals) > 0 {
		for _, p := range lp.Proposals {
			proposals[p.ID] = seenRecord{title: p.Title, state: p.State}
		}
	}

	// list_runs. Each row may also carry an issue_id (+ title/state of
	// the linked proposal); when present, also stash the proposal so
	// the model can validly pivot from a run record to its proposal
	// reference (DSGWOO-1362).
	var lr struct {
		Runs []struct {
			ID         string `json:"id"`
			Persona    string `json:"persona"`
			Status     string `json:"status"`
			IssueID    string `json:"issue_id,omitempty"`
			IssueTitle string `json:"issue_title,omitempty"`
			IssueState string `json:"issue_state,omitempty"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(raw), &lr); err == nil && len(lr.Runs) > 0 {
		for _, r := range lr.Runs {
			runs[r.ID] = seenRecord{title: r.Persona + " run", state: r.Status}
			if r.IssueID != "" {
				if _, already := proposals[r.IssueID]; !already {
					proposals[r.IssueID] = seenRecord{title: r.IssueTitle, state: r.IssueState}
				}
			}
		}
	}

	// get_proposal (singular)
	var gp struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Persona string `json:"persona"`
	}
	if err := json.Unmarshal([]byte(raw), &gp); err == nil && gp.ID != "" && gp.Title != "" {
		proposals[gp.ID] = seenRecord{title: gp.Title, state: gp.State}
	}

	// get_run (singular) — same id field, no title, has status.
	var gr struct {
		ID      string `json:"id"`
		Persona string `json:"persona"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &gr); err == nil && gr.ID != "" && gr.Status != "" && gr.Persona != "" {
		// Avoid double-recording: if get_proposal already claimed this
		// id, leave it. Otherwise stash as a run.
		if _, isProp := proposals[gr.ID]; !isProp {
			runs[gr.ID] = seenRecord{title: gr.Persona + " run", state: gr.Status}
		}
	}

	// dispatch_persona (success path)
	var dp struct {
		OK         bool   `json:"ok"`
		Persona    string `json:"persona"`
		RunID      string `json:"run_id"`
		ETASeconds int    `json:"eta_seconds"`
		Target     string `json:"target"`
	}
	if err := json.Unmarshal([]byte(raw), &dp); err == nil && dp.OK && dp.RunID != "" {
		*dispatched = append(*dispatched, Dispatched{
			Persona:    dp.Persona,
			RunID:      dp.RunID,
			ETASeconds: dp.ETASeconds,
		})
		// Also index the run id so the model's `[run rn_X]` chip can
		// resolve its title/state without a separate get_run.
		title := dp.Persona + " run"
		if dp.Target != "" {
			title = dp.Persona + " on " + dp.Target
		}
		runs[dp.RunID] = seenRecord{title: title, state: "working"}
	}
}
