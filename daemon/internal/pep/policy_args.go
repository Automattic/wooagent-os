package pep

import "encoding/json"

// maxArgsBytes is the upper bound on serialized Args size. 256 KiB is
// well clear of the largest legitimate today-payload (a product
// description rewrite is ~1-2 KiB) while catching runaway prompts and
// bloated targets. Tune if a legitimate use case approaches the ceiling.
const maxArgsBytes = 256 * 1024

// defensiveArgsPolicy gates on argument shape, not content. Fires on
// every Invoke today and catches bugs in dispatcher code that builds
// malformed params maps. Cheapest of the built-ins.
type defensiveArgsPolicy struct{}

func (*defensiveArgsPolicy) Name() string { return "defensive_args" }

func (*defensiveArgsPolicy) Evaluate(req Request, _ EvalContext) (bool, string) {
	if req.Intent == IntentApply && len(req.Args) == 0 {
		return true, "apply intent requires non-empty arguments"
	}
	// Re-marshal Args for the size check. The existing hashArgs already
	// marshals, but plumbing the byte length out of there would couple
	// the audit hashing API to this policy. Marshal cost on a normal
	// request is microseconds; on an oversized request the marshal IS
	// the gate, so we want it to run.
	raw, err := json.Marshal(req.Args)
	if err != nil {
		// Args that can't be marshalled are themselves malformed —
		// fail-closed.
		return true, "arguments could not be serialized for size check"
	}
	if len(raw) > maxArgsBytes {
		return true, "arguments payload exceeds 256 KiB"
	}
	return false, ""
}
