import { useEffect, useState } from 'react';
import { Modal, Button, Notice, Spinner } from '@wordpress/components';
import { Stack, Text } from '@wordpress/ui';

import { ApiError, api, type Connection, type Persona } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from './PersonaAvatar';

interface Props {
  connection: Connection;
  /** The current agents roster from /v1/agents. Used to compute which
      personas are still addable (registered + enabled=false). */
  agents: Persona[];
  /** Called after a successful enable so the parent screen can refetch
      and the modal closes. */
  onAgentAdded: () => void;
  onClose: () => void;
}

interface AddableEntry {
  /** Daemon slug (e.g. "reporting", "chief-of-staff"). Source of truth
      for the enable POST. PersonaAvatar's 2-letter key is derived from
      this via personaKeyFrom. */
  slug: string;
  name: string;
  description: string;
  /** "available" -> daemon has the persona registered, enable button is
      live. "coming-soon" -> persona isn't built yet; rendered with a
      disabled button. */
  status: 'available' | 'coming-soon';
}

// Static description text per persona — daemon doesn't carry these
// today, and they're operator-facing copy that benefits from being
// editorial rather than mechanical. Keep in sync with the Agents page's
// PERSONA_META mandate strings when they overlap.
const PERSONA_COPY: Record<string, { name: string; description: string }> = {
  reporting: {
    name: 'Reporting',
    description:
      "Produces digests of catalog issues that need your attention — starting with products missing short or long descriptions. Read it on your own cadence; dismiss what's intentional.",
  },
  inventory: {
    name: 'Inventory',
    description:
      'Watches stock levels, flags low-stock and overstock situations, and drafts reorder proposals against your supplier list.',
  },
  accounting: {
    name: 'Accounting',
    description:
      'Reconciles WooPayments and Stripe payouts against your bank, flags tax-relevant changes, and drafts month-end summaries.',
  },
  'chief-of-staff': {
    name: 'Chief of Staff',
    description:
      "Triages incoming work across your fleet, routes it to the right specialist, and keeps you briefed on what's drafted, approved, and waiting.",
  },
};

const COMING_SOON_SLUGS = ['inventory', 'accounting', 'chief-of-staff'];

function buildAddable(agents: Persona[]): AddableEntry[] {
  const enabledSlugs = new Set(agents.filter((a) => a.enabled).map((a) => a.persona));
  const out: AddableEntry[] = [];

  // Personas the daemon registers but hasn't been enabled — surface as
  // "Add" buttons. Today: just Reporting.
  for (const a of agents) {
    if (a.enabled) continue;
    const copy = PERSONA_COPY[a.persona];
    if (!copy) continue; // unknown slug, skip to avoid surprising the operator
    out.push({
      slug: a.persona,
      name: copy.name,
      description: copy.description,
      status: 'available',
    });
  }

  // Hard-coded coming-soon personas — these aren't registered server-side
  // yet, so they wouldn't show up in /v1/agents at all. The modal is the
  // honest place to surface "your fleet can include these once we ship
  // them" without faking a daemon row.
  for (const slug of COMING_SOON_SLUGS) {
    if (enabledSlugs.has(slug)) continue;
    // If a coming-soon persona is somehow already an "available" entry
    // above (daemon registered it before the UI caught up), skip the
    // hard-coded version so we don't double-list.
    if (out.some((o) => o.slug === slug)) continue;
    const copy = PERSONA_COPY[slug];
    if (!copy) continue;
    out.push({
      slug,
      name: copy.name,
      description: copy.description,
      status: 'coming-soon',
    });
  }

  return out;
}

export default function AddAgentModal({ connection, agents, onAgentAdded, onClose }: Props) {
  const [pendingSlug, setPendingSlug] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [addable, setAddable] = useState<AddableEntry[]>(() => buildAddable(agents));

  // Recompute when the parent passes a fresh agents list (e.g. after a
  // successful enable propagates back).
  useEffect(() => {
    setAddable(buildAddable(agents));
  }, [agents]);

  async function handleAdd(entry: AddableEntry) {
    if (entry.status !== 'available') return;
    setPendingSlug(entry.slug);
    setError(null);
    try {
      await api.patchAgent(connection, entry.slug, { enabled: true });
      onAgentAdded();
      onClose();
    } catch (e) {
      const message = e instanceof ApiError ? e.message : String(e);
      setError(`Couldn't add ${entry.name}: ${message}`);
      setPendingSlug(null);
    }
  }

  return (
    <Modal
      title="Add an agent to your fleet"
      onRequestClose={onClose}
      size="medium"
      __experimentalHideHeader={false}
    >
      <Stack direction="column" gap="md">
        <Text variant="body-md">
          Your fleet grows as we ship more personas and as plugins on your store
          enable new ones. Pick an agent to add — it'll start drafting work on
          its own cadence and land proposals in your queue for review.
        </Text>

        {error && (
          <Notice status="error" isDismissible onRemove={() => setError(null)}>
            {error}
          </Notice>
        )}

        {addable.length === 0 ? (
          <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-muted)' }}>
            Every available agent is already on your fleet. Future personas
            will show up here as we ship them.
          </Text>
        ) : (
          <Stack direction="column" gap="sm">
            {addable.map((entry) => (
              <AddableCard
                key={entry.slug}
                entry={entry}
                isPending={pendingSlug === entry.slug}
                disabledForPending={pendingSlug !== null && pendingSlug !== entry.slug}
                onAdd={() => handleAdd(entry)}
              />
            ))}
          </Stack>
        )}
      </Stack>
    </Modal>
  );
}

interface CardProps {
  entry: AddableEntry;
  isPending: boolean;
  /** True when another card's Add is in flight — disables this card's
      button so the operator can't fire two enables in parallel. */
  disabledForPending: boolean;
  onAdd: () => void;
}

function AddableCard({ entry, isPending, disabledForPending, onAdd }: CardProps) {
  const isAvailable = entry.status === 'available';
  return (
    <div
      style={{
        display: 'flex',
        gap: 16,
        padding: 'var(--wpds-dimension-padding-md)',
        border: '1px solid var(--wpds-color-stroke-subtle)',
        borderRadius: 'var(--wpds-border-radius-md)',
        background: isAvailable
          ? 'var(--wpds-color-bg-surface-default)'
          : 'var(--wpds-color-bg-surface-muted)',
      }}
    >
      <PersonaAvatar persona={personaKeyFrom(entry.slug)} size="md" />
      <div style={{ flex: 1, minWidth: 0 }}>
        <Stack direction="column" gap="xs">
          <Text
            variant="body-md"
            style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
          >
            {entry.name}
            {!isAvailable && (
              <span
                style={{
                  marginLeft: 8,
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  fontWeight: 'var(--wpds-typography-font-weight-regular)',
                  color: 'var(--wpds-color-fg-content-muted)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.04em',
                }}
              >
                Coming soon
              </span>
            )}
          </Text>
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-muted)' }}
          >
            {entry.description}
          </Text>
        </Stack>
      </div>
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <Button
          variant={isAvailable ? 'primary' : 'tertiary'}
          disabled={!isAvailable || isPending || disabledForPending}
          onClick={onAdd}
          __next40pxDefaultSize
        >
          {isPending ? (
            <>
              <Spinner /> Adding…
            </>
          ) : isAvailable ? (
            'Add to fleet'
          ) : (
            'Coming soon'
          )}
        </Button>
      </div>
    </div>
  );
}
