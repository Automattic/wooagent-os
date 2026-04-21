package telemetry

import "time"

// EventSchemaVersion is the running counter for TurnEvent schema changes. Bump
// when any field name/type changes in a way a consumer would notice. Consumers
// (the central GEPA ingest pipeline) filter by this on read.
const EventSchemaVersion = 1

// TurnEvent is one agent turn: the model calls, skill calls, proposal, and
// operator verdict that resulted from one issue tick. This is the row shape
// the v2 GEPA pipeline ingests; see wooagent-os-gepa-v2-plan.md §"Structured
// turn telemetry" for rationale. Nail this schema during v1 — changing it
// after operators have accumulated data means painful migrations.
type TurnEvent struct {
	TurnID              string        `json:"turn_id"`
	EventSchemaVersion  int           `json:"event_schema_version"`
	IssueID             string        `json:"issue_id,omitempty"`
	Persona             string        `json:"persona,omitempty"`
	PromptVersion       string        `json:"prompt_version,omitempty"`
	SkillVersions       []SkillRef    `json:"skill_versions,omitempty"`
	StartedAt           time.Time     `json:"started_at"`
	CompletedAt         *time.Time    `json:"completed_at,omitempty"`
	LatencyMS           int64         `json:"latency_ms,omitempty"`
	Context             TurnContext   `json:"context"`
	ModelCalls          []ModelCall   `json:"model_calls,omitempty"`
	SkillCalls          []SkillCall   `json:"skill_calls,omitempty"`
	ProposalText        string        `json:"proposal_text,omitempty"`
	ProposalSHA         string        `json:"proposal_sha,omitempty"`
	Verdict             *Verdict      `json:"verdict,omitempty"`
}

type SkillRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type TurnContext struct {
	IssueSummary       string `json:"issue_summary,omitempty"`
	StoreSnapshotHash  string `json:"store_snapshot_hash,omitempty"`
	OperatorIntent     string `json:"operator_intent,omitempty"`
}

type ModelCall struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	RequestSHA   string `json:"request_sha,omitempty"`
	ResponseSHA  string `json:"response_sha,omitempty"`
}

type SkillCall struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ArgsSHA   string `json:"args_sha,omitempty"`
	Status    string `json:"status"` // ok | error
	LatencyMS int64  `json:"latency_ms,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// VerdictKind captures operator verdict; stays in sync with the reason-tag
// taxonomy in wooagent-os-gepa-v2-plan.md §"Outcome metrics".
type VerdictKind string

const (
	VerdictApprove           VerdictKind = "approve"
	VerdictApproveWithEdits  VerdictKind = "approve_with_edits"
	VerdictReject            VerdictKind = "reject"
)

type Verdict struct {
	Kind           VerdictKind `json:"kind"`
	ReasonTag      string      `json:"reason_tag,omitempty"`  // off-brand | wrong-numbers | irrelevant | unsafe | incomplete | other
	ReasonText     string      `json:"reason_text,omitempty"`
	PublishedText  string      `json:"published_text,omitempty"`
	PublishedSHA   string      `json:"published_sha,omitempty"`
	DecidedAt      time.Time   `json:"decided_at"`
}
