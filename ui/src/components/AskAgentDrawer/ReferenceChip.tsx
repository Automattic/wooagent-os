import { useNavigate } from 'react-router-dom';
import type { AskReference } from '../../api/client';

interface Props {
  reference: AskReference;
  onNavigated: () => void;
}

// ReferenceChip turns a structured AskReference into a clickable pill.
// Clicking navigates to the proposal or run and closes the drawer so
// the operator lands directly on the linked thing — the chat history
// remains intact via the in-memory message state.
//
// CUSTOM: pill-shaped reference chip with kind-tagged content. (a)
// WPDS has no chip / tag component that fits "agent reference" use —
// Badge is the closest but reads as static state, not navigation. (b)
// Inline-flex button styled to match the drawer surface tone, navigates
// on click via react-router-dom. (c) Will collapse into a shared chip
// primitive if a third agent-output surface needs the same affordance.
export default function ReferenceChip({ reference, onNavigated }: Props) {
  const navigate = useNavigate();
  const label = reference.title || `${reference.kind} ${reference.id}`;
  const path =
    reference.kind === 'proposal'
      ? `/issues/${reference.id}`
      : reference.kind === 'run'
        ? `/runs/${reference.id}`
        : `/agents/${reference.id}`;

  return (
    <button
      type="button"
      className="wa-ref-chip"
      onClick={() => {
        navigate(path);
        onNavigated();
      }}
      title={
        reference.state ? `${reference.kind} · ${reference.state}` : reference.kind
      }
    >
      <span className="wa-ref-chip__kind">{reference.kind}</span>
      <span className="wa-ref-chip__label">{label}</span>
    </button>
  );
}
