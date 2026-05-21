import type { AskAgent } from '../../api/client';

interface Props {
  active: AskAgent;
}

// Picker is the agent selector at the top of the drawer. In B1 it
// only shows the currently active agent (Chief of Staff) — the
// interactive switching + disabled-specialist tooltip lands in B2
// (DSGWOO-1348). The visual scaffold is here so the drawer doesn't
// look like its picker just appeared mid-development.
//
// CUSTOM: agent-label header strip. (a) WPDS SelectControl is the
// long-term home for this affordance but B1 doesn't have switchable
// agents yet — rendering a disabled Select would set the wrong
// expectation (operator thinks it's a bug they can't click it).
// (b) A static labeled row matching the drawer-header tone. (c) Will
// be replaced by an interactive SelectControl + persona avatars in B2.
const LABELS: Record<AskAgent, string> = {
  chief_of_staff: 'Chief of Staff',
  marketing: 'Marketing',
  pricing: 'Pricing',
  'sales-support': 'Sales Support',
};

export default function Picker({ active }: Props) {
  return (
    <div className="wa-chat-picker" aria-label="Active agent">
      <span className="wa-chat-picker__label">Agent</span>
      <span className="wa-chat-picker__name">{LABELS[active]}</span>
    </div>
  );
}
