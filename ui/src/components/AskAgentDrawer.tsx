import { useEffect, useRef } from 'react';
import { Icon, close, plus, arrowUp, check, chartBar, page, columns } from '@wordpress/icons';
import { Stack, Text } from '@wordpress/ui';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  /** Page-context line shown under the input — e.g. "Board · Today's marketing queue · 9 items". */
  contextLabel: string;
}

// Stub command-palette drawer. V1 renders the shell + suggestions but doesn't
// route queries anywhere yet; the input is decorative. Wires up the ⌘K
// shortcut, focus-on-open, and Esc-to-close so the affordance is real.
export default function AskAgentDrawer({ isOpen, onClose, contextLabel }: Props) {
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (isOpen) inputRef.current?.focus();
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isOpen, onClose]);

  return (
    <>
      <div
        className={`wa-drawer-scrim${isOpen ? ' is-open' : ''}`}
        onClick={onClose}
        aria-hidden="true"
      />
      <aside
        className={`wa-drawer${isOpen ? ' is-open' : ''}`}
        role="dialog"
        aria-label="Ask agent"
        aria-hidden={!isOpen}
      >
        <div className="wa-drawer__header">
          <Text variant="heading-sm">Ask agent</Text>
          {/* CUSTOM: drawer-chrome close button using shared .wa-icon-btn class. (a) WPDS <Button icon={close} variant="tertiary"> doesn't match drawer-header size/padding. (b) icon-only close with .wa-icon-btn shared chrome. (c) Follow-up: migrate when .wa-icon-btn retires. */}
          <button
            type="button"
            className="wa-icon-btn"
            onClick={onClose}
            aria-label="Close"
          >
            <Icon icon={close} size={18} />
          </button>
        </div>

        <div className="wa-drawer__body">
          <label className="wa-topbar__search" style={{ width: '100%' }}>
            <Icon icon={plus} size={16} />
            {/* CUSTOM: compound input with embedded leading icon + trailing esc kbd hint inside one .wa-topbar__search shell. (a) WPDS SearchControl/InputControl don't expose a trailing slot for kbd hints. (b) <input> wrapped in a <label> with side icons. (c) Follow-up: revisit if WPDS adds input-slots. */}
            <input
              ref={inputRef}
              type="text"
              placeholder="Ask anything about this queue…"
              aria-label="Ask agent"
            />
            <span className="wa-kbd">esc</span>
          </label>

          <div
            style={{
              padding: 'var(--wpds-dimension-padding-sm) var(--wpds-dimension-padding-md)',
              background: 'var(--wpds-color-bg-surface-info-weak)',
              borderRadius: 'var(--wpds-border-radius-md)',
              fontSize: 'var(--wpds-typography-font-size-sm)',
              color: 'var(--wpds-color-fg-interactive-brand)',
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--wpds-dimension-gap-sm)',
            }}
          >
            <Icon icon={columns} size={16} />
            {contextLabel}
          </div>

          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            Open anytime with <span className="wa-kbd">⌘K</span>
          </Text>

          <Stack direction="column" gap="xs">
            <span className="wa-eyebrow">Suggested for this page</span>
            <div>
              <Suggestion
                icon={arrowUp}
                label="Which item should I review first for maximum impact?"
                highlighted
              />
              <Suggestion
                icon={page}
                label="Why has Handwoven Wool Throw been in review for 8 minutes?"
              />
              <Suggestion
                icon={check}
                label="What can I quickly approve to clear the queue?"
              />
              <Suggestion
                icon={chartBar}
                label="Give me a one-sentence summary of today's queue."
              />
            </div>
          </Stack>

          <Stack direction="column" gap="xs">
            <span className="wa-eyebrow">Recent</span>
            <div>
              <Suggestion
                icon={page}
                label="Approved Variant A for Linen Napkin"
                meta="2 days ago"
              />
              <Suggestion
                icon={page}
                label="Rejected draft for Mother's Day micro-campaign"
                meta="5 days ago"
              />
            </div>
          </Stack>
        </div>

        <div className="wa-drawer__footer">
          <span>
            <span className="wa-kbd">↑↓</span> navigate
          </span>
          <span>
            <span className="wa-kbd">↵</span> select
          </span>
          <span>
            <span className="wa-kbd">esc</span> dismiss
          </span>
        </div>
      </aside>
    </>
  );
}

interface SuggestionProps {
  icon: { type: string } | unknown;
  label: string;
  meta?: string;
  highlighted?: boolean;
}

function Suggestion({ icon, label, meta, highlighted }: SuggestionProps) {
  return (
    <div
      className={`wa-suggestion-row${highlighted ? ' wa-suggestion-row--highlight' : ''}`}
    >
      <Icon icon={icon as never} size={16} />
      <span className="wa-suggestion-row__label">{label}</span>
      {meta && <span className="wa-suggestion-row__time">{meta}</span>}
    </div>
  );
}
