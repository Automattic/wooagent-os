import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Spinner } from '@wordpress/components';
import { api, type AskDispatched, type Connection, type Run } from '../../api/client';

interface Props {
  dispatched: AskDispatched;
  connection: Connection;
  onNavigated: () => void;
}

const PERSONA_LABELS: Record<string, string> = {
  marketing: 'Marketing',
  pricing: 'Pricing',
  'sales-support': 'Sales Support',
};

/** Lifecycle states the chip surfaces. `working` is the initial state
 *  right after dispatch; the poller transitions to one of the terminal
 *  states (`succeeded`, `failed`, `skipped`) when the run completes. */
type ChipState =
  | { kind: 'working' }
  | { kind: 'succeeded'; issueId: string | null; title: string }
  | { kind: 'failed'; reason: string }
  | { kind: 'skipped'; reason: string };

/** Runs poll cadence — once at 5s, then every 5s up to 90s total.
 *  Matches the issue spec; the cadence-mode runtimes target Marketing
 *  ~10s, Pricing ~60s, Sales Support ~15s, so 5s buckets cover all
 *  three without hammering the daemon. */
const POLL_INTERVAL_MS = 5000;
const POLL_TIMEOUT_MS = 90_000;

// DispatchChip is the receipt for a dispatch_persona tool call. While
// the run is in flight it renders as a "Working" chip with a spinner;
// once the poller observes the run reach a terminal state, the chip
// rewrites itself to the proposal title (on success) or failure reason.
// Clicking always navigates to the run-detail screen — even on success
// the operator can see the trace, and the resulting proposal is also
// linked on the board separately.
//
// CUSTOM: chip variant with a Spinner + live status. (a) Distinct from
// ReferenceChip so the operator can tell at a glance that the chip is
// in-progress vs settled. (b) Inline-flex button with WPDS Spinner +
// label. (c) Documented alongside other drawer-chrome customs in
// DESIGN.md (DSGWOO-1356).
export default function DispatchChip({ dispatched, connection, onNavigated }: Props) {
  const navigate = useNavigate();
  const personaLabel = PERSONA_LABELS[dispatched.persona] ?? dispatched.persona;
  const [state, setState] = useState<ChipState>({ kind: 'working' });

  useEffect(() => {
    if (state.kind !== 'working') return;
    let cancelled = false;
    const startedAt = Date.now();
    let timerId: number | undefined;

    const tick = async () => {
      try {
        const { run } = await api.runs.get(connection, dispatched.run_id);
        if (cancelled) return;
        const terminal = terminalStateFor(run);
        if (terminal) {
          if (terminal.kind === 'succeeded' && run.issue_id) {
            // Fetch the resulting issue title so the chip can settle
            // with something specific. Best-effort: if this fails,
            // fall back to a generic "Done" label.
            try {
              const detail = await api.issue(connection, run.issue_id);
              if (!cancelled) {
                setState({
                  kind: 'succeeded',
                  issueId: run.issue_id,
                  title: detail.issue.title,
                });
              }
            } catch {
              if (!cancelled) {
                setState({ kind: 'succeeded', issueId: run.issue_id, title: 'Done' });
              }
            }
          } else {
            setState(terminal);
          }
          return;
        }
      } catch {
        // Poll failure (network blip, daemon transient) — drop and let
        // the next tick try again. Don't surface to the user; the chip
        // staying "working" is the right signal.
      }
      if (cancelled) return;
      if (Date.now() - startedAt >= POLL_TIMEOUT_MS) {
        // Gave up polling. Leave chip as "working" — the operator can
        // still click through and the board will surface the eventual
        // proposal even without a chip update.
        return;
      }
      timerId = window.setTimeout(tick, POLL_INTERVAL_MS);
    };
    // First tick after POLL_INTERVAL_MS — matches the spec ("once at
    // 5s, then every 5s") and gives the scheduler a beat to claim
    // the run before the first poll lands.
    timerId = window.setTimeout(tick, POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      if (timerId !== undefined) window.clearTimeout(timerId);
    };
  }, [connection, dispatched.run_id, state.kind]);

  const onClick = () => {
    if (state.kind === 'succeeded' && state.issueId) {
      navigate(`/issues/${state.issueId}`);
    } else {
      navigate(`/runs/${dispatched.run_id}`);
    }
    onNavigated();
  };

  if (state.kind === 'working') {
    return (
      <button
        type="button"
        className="wa-ref-chip wa-ref-chip--working"
        onClick={onClick}
        title={`${personaLabel} is working — about ${dispatched.eta_seconds}s`}
      >
        <span className="wa-ref-chip__spinner" aria-hidden="true">
          <Spinner />
        </span>
        <span className="wa-ref-chip__label">{personaLabel} is working…</span>
      </button>
    );
  }
  if (state.kind === 'succeeded') {
    return (
      <button
        type="button"
        className="wa-ref-chip"
        onClick={onClick}
        title={`${personaLabel} · ${state.title}`}
      >
        <span className="wa-ref-chip__label">{state.title}</span>
      </button>
    );
  }
  // failed or skipped
  return (
    <button
      type="button"
      className="wa-ref-chip wa-ref-chip--failed"
      onClick={onClick}
      title={state.reason}
    >
      <span className="wa-ref-chip__label">
        {state.kind === 'failed' ? `Failed: ${state.reason}` : `Skipped: ${state.reason}`}
      </span>
    </button>
  );
}

/** Returns the terminal state for a run, or null if still in flight.
 *  Daemon RunStatus values: queued | running | succeeded | skipped |
 *  failed | failed_permanent. failed_permanent collapses into failed
 *  for the chip — the operator doesn't care about the distinction at
 *  the chat-receipt level. */
function terminalStateFor(run: Run): ChipState | null {
  switch (run.status) {
    case 'succeeded':
      return { kind: 'succeeded', issueId: run.issue_id, title: 'Done' };
    case 'failed':
    case 'failed_permanent':
      return { kind: 'failed', reason: run.failure_reason ?? 'unknown failure' };
    case 'skipped':
      return { kind: 'skipped', reason: run.skip_reason ?? 'no proposal' };
    default:
      return null;
  }
}
