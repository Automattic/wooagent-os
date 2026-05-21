import { Dropdown } from '@wordpress/components';
import { Icon, chevronDown } from '@wordpress/icons';
import { PersonaAvatar, type PersonaKey } from '../PersonaAvatar';
import type { AskAgent } from '../../api/client';

interface Props {
  active: AskAgent;
  onSelect: (next: AskAgent) => void;
  /** Map of agent → unread count (>0 when a thread the operator isn't
   *  viewing got a new message — e.g., dispatched run completed while
   *  the operator was in another thread). Picker shows a dot indicator
   *  on the avatar; numeric badges are out of scope for v1. */
  unread?: Partial<Record<AskAgent, boolean>>;
}

interface AgentRow {
  key: AskAgent | DisabledKey;
  label: string;
  persona: PersonaKey;
  disabled: boolean;
  disabledReason?: string;
}

/** Internal sentinel for non-selectable picker rows. These appear in
 *  the UI but can never become the active agent. */
type DisabledKey = 'inventory' | 'accounting' | 'reporting';

/** Canonical persona order — every UI that enumerates the seven personas
 *  lists them this way (Marketing → Pricing → Inventory → Accounting →
 *  Reporting → Sales Support → Chief of Staff). Chief of Staff is pinned
 *  to the bottom slot even though it's pre-selected by default. */
const AGENT_ROWS: AgentRow[] = [
  { key: 'marketing',     label: 'Marketing',      persona: 'mk', disabled: false },
  { key: 'pricing',       label: 'Pricing',        persona: 'pr', disabled: false },
  { key: 'inventory',     label: 'Inventory',      persona: 'in', disabled: true, disabledReason: 'Coming soon' },
  { key: 'accounting',    label: 'Accounting',     persona: 'ac', disabled: true, disabledReason: 'Coming soon' },
  { key: 'reporting',     label: 'Reporting',      persona: 'rp', disabled: true, disabledReason: 'Coming soon' },
  { key: 'sales-support', label: 'Sales Support',  persona: 'ss', disabled: false },
  { key: 'chief_of_staff',label: 'Chief of Staff', persona: 'cs', disabled: false },
];

function rowFor(key: AskAgent): AgentRow {
  return AGENT_ROWS.find((r) => r.key === key)!;
}

export default function Picker({ active, onSelect, unread }: Props) {
  const activeRow = rowFor(active);

  return (
    <div className="wa-chat-picker">
      <Dropdown
        popoverProps={{ placement: 'bottom-start', offset: 4 }}
        renderToggle={({ isOpen, onToggle }) => (
          // CUSTOM: drawer-header agent toggle. (a) WPDS SelectControl can't
          // render the persona avatar inside its select chrome — it draws a
          // native <select>. (b) Custom button with PersonaAvatar + label +
          // chevron, styled with .wa-chat-picker__toggle to match the drawer
          // header tone. (c) Documented in DESIGN.md alongside the other
          // drawer-chrome customs (close button, suggestion rows).
          <button
            type="button"
            className="wa-chat-picker__toggle"
            aria-expanded={isOpen}
            aria-haspopup="listbox"
            aria-label={`Active agent: ${activeRow.label}. Click to switch.`}
            onClick={onToggle}
          >
            <PersonaAvatar persona={activeRow.persona} size="sm" />
            <span className="wa-chat-picker__name">{activeRow.label}</span>
            <Icon icon={chevronDown} size={16} />
          </button>
        )}
        renderContent={({ onClose }) => (
          <div className="wa-chat-picker__menu" role="listbox">
            {AGENT_ROWS.map((row) => {
              const isActive = !row.disabled && row.key === active;
              const isUnread = !row.disabled && unread?.[row.key as AskAgent];
              return (
                <button
                  key={row.key}
                  type="button"
                  role="option"
                  aria-selected={isActive}
                  aria-disabled={row.disabled}
                  disabled={row.disabled}
                  title={row.disabledReason}
                  className={
                    'wa-chat-picker__option' +
                    (isActive ? ' is-active' : '') +
                    (row.disabled ? ' is-disabled' : '')
                  }
                  onClick={() => {
                    if (row.disabled) return;
                    onSelect(row.key as AskAgent);
                    onClose();
                  }}
                >
                  <span className="wa-chat-picker__option-avatar">
                    <PersonaAvatar persona={row.persona} size="sm" />
                    {isUnread && <span className="wa-chat-picker__unread-dot" aria-hidden="true" />}
                  </span>
                  <span className="wa-chat-picker__option-label">{row.label}</span>
                  {row.disabled && (
                    <span className="wa-chat-picker__option-hint">{row.disabledReason}</span>
                  )}
                </button>
              );
            })}
          </div>
        )}
      />
    </div>
  );
}
