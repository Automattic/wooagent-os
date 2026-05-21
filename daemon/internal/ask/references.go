package ask

import (
	"encoding/json"
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
// blocks, then matches them against the references the model actually
// cited in the final assistant text.
//
// Returned references appear in citation order (first mention wins);
// duplicates within the text are deduped on id. Dispatched receipts
// come from every successful dispatch_persona call across the trace,
// regardless of whether the model mentioned them.
func ExtractReferencesAndDispatched(finalText string, trace anthropic.Trace) ([]Reference, []Dispatched) {
	proposalsByID, runsByID, dispatchedList := scanTrace(trace)

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
		rec := proposalsByID[id]
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
		rec := runsByID[id]
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

	// list_runs
	var lr struct {
		Runs []struct {
			ID      string `json:"id"`
			Persona string `json:"persona"`
			Status  string `json:"status"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(raw), &lr); err == nil && len(lr.Runs) > 0 {
		for _, r := range lr.Runs {
			runs[r.ID] = seenRecord{title: r.Persona + " run", state: r.Status}
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
