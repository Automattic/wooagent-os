import { useNavigate } from 'react-router-dom';
import { Spinner } from '@wordpress/components';
import type { AskDispatched } from '../../api/client';

interface Props {
  dispatched: AskDispatched;
  onNavigated: () => void;
}

const PERSONA_LABELS: Record<string, string> = {
  marketing: 'Marketing',
  pricing: 'Pricing',
  'sales-support': 'Sales Support',
};

// DispatchChip is the receipt for a dispatch_persona tool call. Looks
// like a reference chip but carries a live "Working" indicator while
// the persona run is in flight. Clicking navigates to the run detail
// screen (the proposal it produces will appear on the board when the
// run completes — that surfaces separately, no chip update needed).
//
// CUSTOM: chip variant with a Spinner glyph. (a) Distinct from
// ReferenceChip so the operator can tell at a glance that the chip is
// in-progress vs settled. (b) Inline-flex button with a Spinner from
// @wordpress/components and the persona label. (c) Live status
// transitions (polling /v1/runs/{id} → reading "succeeded" → replacing
// the spinner with the resulting proposal title) are deferred to a
// follow-up under the SSE thinking-events work (DSGWOO-1348 A4 + B3).
export default function DispatchChip({ dispatched, onNavigated }: Props) {
  const navigate = useNavigate();
  const personaLabel = PERSONA_LABELS[dispatched.persona] ?? dispatched.persona;

  return (
    <button
      type="button"
      className="wa-ref-chip wa-ref-chip--working"
      onClick={() => {
        navigate(`/runs/${dispatched.run_id}`);
        onNavigated();
      }}
      title={`${personaLabel} is working — about ${dispatched.eta_seconds}s`}
    >
      <span className="wa-ref-chip__spinner" aria-hidden="true">
        <Spinner />
      </span>
      <span className="wa-ref-chip__label">{personaLabel} is working…</span>
    </button>
  );
}
