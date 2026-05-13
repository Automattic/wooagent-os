import { useEffect, useState } from 'react';
import { Dialog, Stack, Text } from '@wordpress/ui';
import { Button, TextareaControl } from '@wordpress/components';
import type { DismissReason } from '../api/client';
import { kindFromPersonaSlug, type IssueKind } from './StatusBadge';

interface Props {
  open: boolean;
  onOpenChange(open: boolean): void;
  /** Persona slug — drives the agent-name copy in the body. */
  persona?: string;
  /** Number of variants under the proposal. Drives the title pluralization
   *  ("Dismiss all variants?" vs "Dismiss this draft?"). */
  variantCount?: number;
  onConfirm(payload: { reason: DismissReason; comment?: string }): void;
  /** True while the dismiss request is in-flight. Disables Confirm + Cancel. */
  busy?: boolean;
}

// Preset reason chips shown at the top of the dialog. Free-text comment
// captures any nuance the operator wants to add. Reason vocabulary aligns
// with the DismissReason union in api/client.ts. The Archive screen displays
// these as the row's "Reason" chip; the comment surfaces in IssueDetail.
const REASON_CHIPS: { value: DismissReason; label: string }[] = [
  { value: 'tone_off', label: 'Tone is off' },
  { value: 'wrong_product_focus', label: 'Wrong product focus' },
  { value: 'not_needed_now', label: 'Not needed now' },
  { value: 'write_myself', label: "I'll write this myself" },
];

const PERSONA_DISPLAY: Record<string, string> = {
  marketing: 'Marketing',
  pricing: 'Pricing',
  sales_support: 'Sales support',
  'sales-support': 'Sales support',
};

// Kind-aware copy for the dialog's title and "won't re-propose…" body line.
// content/campaign/email → variants (Marketing); price → a price proposal;
// message → a reply draft.
function titleFor(kind: IssueKind, variantCount: number): string {
  if (kind === 'price') return 'Dismiss this price proposal?';
  if (kind === 'message') return 'Dismiss this draft?';
  return variantCount > 1 ? 'Dismiss all variants?' : 'Dismiss this draft?';
}

function bodyFor(agent: string, kind: IssueKind): string {
  const noun =
    kind === 'price' ? 'price changes' : kind === 'message' ? 'replies' : 'copy';
  const subject =
    kind === 'price' || kind === 'message' ? 'this item' : 'this product';
  return `The ${agent} agent won't re-propose ${noun} for ${subject} unless you ask. ${kind === 'price' || kind === 'message' ? 'This' : 'These variants'} move${kind === 'price' || kind === 'message' ? 's' : ''} to your archive — not deleted yet.`;
}

function formatDeletionDate(): string {
  const d = new Date();
  d.setDate(d.getDate() + 30);
  return d.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

export default function DismissDialog({
  open,
  onOpenChange,
  persona,
  variantCount = 1,
  onConfirm,
  busy = false,
}: Props) {
  const [reason, setReason] = useState<DismissReason | null>(null);
  const [comment, setComment] = useState('');

  // Reset selection each time the dialog is freshly opened so the operator
  // doesn't see last session's chip still highlighted.
  useEffect(() => {
    if (open) {
      setReason(null);
      setComment('');
    }
  }, [open]);

  const kind = kindFromPersonaSlug(persona);
  const agentDisplay = persona ? PERSONA_DISPLAY[persona] ?? persona : 'Agent';
  const deletionDate = formatDeletionDate();
  const title = titleFor(kind, variantCount);
  const body = bodyFor(agentDisplay, kind);

  const handleConfirm = () => {
    if (!reason) return;
    onConfirm({ reason, comment: comment.trim() || undefined });
  };

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Popup size="medium">
        <Dialog.Header>
          <Dialog.Title>{title}</Dialog.Title>
        </Dialog.Header>

        <Stack direction="column" gap="lg">
          <Text
            variant="body-sm"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              textWrap: 'pretty',
            }}
          >
            {body}
          </Text>

          <Stack direction="column" gap="sm">
            <Text variant="heading-sm">Tell the agent why</Text>
            {/* CUSTOM: chip group of preset dismiss reasons. (a) WPDS has no
                ToggleGroupControl in @wordpress/ui yet — DSGWOO follow-up.
                (b) Inline wrapping flex of WPDS Buttons; selected reason gets
                primary variant, others secondary. (c) Replace with
                ToggleGroupControl once it ships. */}
            <div className="wa-dismiss-dialog__chips">
              {REASON_CHIPS.map((r) => (
                <Button
                  key={r.value}
                  variant={reason === r.value ? 'primary' : 'secondary'}
                  __next40pxDefaultSize
                  onClick={() => setReason(r.value)}
                >
                  {r.label}
                </Button>
              ))}
            </div>
          </Stack>

          <TextareaControl
            label="Add more context"
            hideLabelFromVision
            placeholder="Optional — add more context for the agent…"
            value={comment}
            onChange={(v: string) => setComment(v)}
            rows={3}
            __nextHasNoMarginBottom
          />

          <div className="wa-dismiss-dialog__info">
            <span aria-hidden="true" className="wa-dismiss-dialog__info-icon">
              📒
            </span>
            <Stack direction="column" gap="xs">
              <Text variant="body-sm">
                Archived for{' '}
                <strong>30 days</strong>
                {' · '}permanently deleted{' '}
                <strong>{deletionDate}</strong>
              </Text>
              <Text
                variant="body-sm"
                style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
              >
                You can restore or extend from the archive at any time.
              </Text>
            </Stack>
          </div>
        </Stack>

        <Dialog.Footer>
          <Dialog.Action
            render={
              <Button variant="tertiary" __next40pxDefaultSize disabled={busy}>
                Cancel
              </Button>
            }
          />
          <Button
            variant="secondary"
            isDestructive
            __next40pxDefaultSize
            disabled={!reason || busy}
            onClick={handleConfirm}
          >
            {busy
              ? 'Dismissing…'
              : variantCount > 1 &&
                  kind !== 'price' &&
                  kind !== 'message'
                ? 'Dismiss all variants'
                : 'Dismiss'}
          </Button>
        </Dialog.Footer>
      </Dialog.Popup>
    </Dialog.Root>
  );
}
