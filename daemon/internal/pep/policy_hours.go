package pep

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// hhmmRe matches "HH:MM" in 24-hour format. Validation is also enforced
// at write time by the agents PATCH handler (via the exported ValidHHMM
// below); we re-check here so a manual SQL edit that bypasses the
// handler can't silently fail open.
var hhmmRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ValidHHMM reports whether s is a well-formed "HH:MM" 24-hour time.
// Exported so HTTP handlers (and any future config validators) share
// the same canonical regex as the operator-hours policy.
func ValidHHMM(s string) bool { return hhmmRe.MatchString(s) }

// operatorHoursPolicy denies agent-sourced apply intents that fall
// outside the configured per-persona window. Both AgentSettings.
// ApplyHoursStart and AgentSettings.ApplyHoursEnd must be set to
// well-formed HH:MM strings or the gate is treated as a no-op (when
// both empty) or fail-closed (when malformed or XOR). Overnight windows
// (end < start) are supported.
type operatorHoursPolicy struct{}

func (*operatorHoursPolicy) Name() string { return "operator_hours" }

func (*operatorHoursPolicy) Evaluate(req Request, ctx EvalContext) (bool, string) {
	if req.Source != SourceAgent || req.Intent != IntentApply {
		return false, ""
	}
	start, end := ctx.Agent.ApplyHoursStart, ctx.Agent.ApplyHoursEnd
	if start == "" && end == "" {
		return false, ""
	}
	if start == "" || end == "" {
		// XOR — operator set one without the other. PATCH handler should
		// reject this; defense in depth here.
		return true, "agent apply blocked: invalid apply-hours config (only one of start/end set)"
	}
	if !hhmmRe.MatchString(start) || !hhmmRe.MatchString(end) {
		return true, "agent apply blocked: invalid apply-hours config (must be HH:MM 24-hour)"
	}
	startMin, err := minutesSinceMidnight(start)
	if err != nil {
		return true, "agent apply blocked: invalid apply-hours config"
	}
	endMin, err := minutesSinceMidnight(end)
	if err != nil {
		return true, "agent apply blocked: invalid apply-hours config"
	}
	now := ctx.Now
	nowMin := now.Hour()*60 + now.Minute()

	insideWindow := false
	if startMin <= endMin {
		// Same-day window, e.g. 09:00-17:00.
		insideWindow = nowMin >= startMin && nowMin < endMin
	} else {
		// Overnight, e.g. 22:00-06:00 means now >= 22:00 OR now < 06:00.
		insideWindow = nowMin >= startMin || nowMin < endMin
	}
	if insideWindow {
		return false, ""
	}
	return true, fmt.Sprintf("agent apply blocked: outside configured hours (%s-%s local)", start, end)
}

func minutesSinceMidnight(hhmm string) (int, error) {
	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("malformed time %q", hhmm)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return h*60 + m, nil
}
