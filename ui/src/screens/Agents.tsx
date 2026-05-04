import { useEffect, useState } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import { Notice, Spinner, FormToggle } from '@wordpress/components';
import { Icon, search, filter, grid, chevronDown, moreVertical } from '@wordpress/icons';
import { api, type Connection, type Persona } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import EditPersonaModal from '../components/EditPersonaModal';

interface Props {
  connection: Connection;
}

// Hardcoded V1 metadata per persona — the daemon doesn't surface mandates,
// run-state, or last-run timestamps yet. Wire to real fields when the daemon
// learns to report them.
interface PersonaMeta {
  mandate: string;
  status: 'running' | 'idle';
  lastRun: string;
  systemPrompt: string;
}

const PERSONA_META: Record<string, PersonaMeta> = {
  marketing: {
    mandate: 'Grows organic traffic and on-site conversion.',
    status: 'running',
    lastRun: '3m ago',
    systemPrompt:
      "You are the Marketing & SEO agent for mystore.com. Your primary goal is to grow organic traffic and improve conversion rates. Monitor keyword rankings, suggest meta description updates, and generate product copy that matches the store's warm, approachable brand voice. Always flag changes before writing to WooCommerce.",
  },
  pricing: {
    mandate: 'Protects margin and monitors competitor pricing.',
    status: 'idle',
    lastRun: '1h ago',
    systemPrompt:
      'You are the Pricing agent. Watch margins, sales velocity, and competitor signals to propose price moves. Always require human approval before changing live prices.',
  },
  inventory: {
    mandate: 'Keeps stock levels healthy; drafts POs.',
    status: 'running',
    lastRun: '6m ago',
    systemPrompt:
      'You are the Inventory agent. Surface low-stock and overstock issues, propose reorder quantities, and draft purchase orders against approved suppliers.',
  },
  accounting: {
    mandate: 'Reconciles payouts, tracks tax, preps books.',
    status: 'idle',
    lastRun: '4h ago',
    systemPrompt:
      'You are the Accounting agent. Reconcile WooPayments and Stripe payouts against the bank, flag tax-relevant changes, and draft month-end summaries.',
  },
  reporting: {
    mandate: 'Produces weekly and monthly digests.',
    status: 'idle',
    lastRun: '4h ago',
    systemPrompt:
      'You are the Reporting agent. Generate weekly and monthly digests covering revenue, conversion, and operational anomalies. Investigate ad-hoc questions on request.',
  },
  'sales-support': {
    mandate: 'Drafts replies to pre-sale and order inquiries.',
    status: 'running',
    lastRun: '12m ago',
    systemPrompt:
      'You are the Sales Support agent. Draft customer replies, handle refund triage, and escalate edge cases. Never reply directly without human approval.',
  },
  chief: {
    mandate: 'Triages, routes, and summarizes across the fleet.',
    status: 'idle',
    lastRun: '2h ago',
    systemPrompt:
      'You are the Chief of Staff. Triage incoming work, route it to the right specialist agent, and keep the operator briefed on what the fleet is doing.',
  },
};

function metaFor(personaKey: string): PersonaMeta {
  return (
    PERSONA_META[personaKey] ?? {
      mandate: '—',
      status: 'idle',
      lastRun: '—',
      systemPrompt: '',
    }
  );
}

// Display name shown in the table — falls back to a polished form if the
// daemon's `name` is plain like "Marketing Agent". Phase-1 only.
function displayName(p: Persona): string {
  if (p.persona === 'marketing') return 'Marketing & SEO';
  if (p.persona === 'inventory') return 'Inventory Manager';
  if (p.persona === 'sales-support') return 'Sales Support';
  if (p.persona === 'chief') return 'Chief of Staff';
  return p.name || p.persona;
}

export default function Agents({ connection }: Props) {
  const [personas, setPersonas] = useState<Persona[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<Persona | null>(null);
  // Local-only enabled state — daemon has no PATCH /v1/agents yet, so toggles
  // don't persist. Initialised from the daemon snapshot.
  const [enabledMap, setEnabledMap] = useState<Record<string, boolean>>({});

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await api.agents(connection);
        if (cancelled) return;
        setPersonas(res.agents);
        const map: Record<string, boolean> = {};
        for (const a of res.agents) map[a.persona] = a.enabled;
        setEnabledMap(map);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  if (error) {
    return (
      <main style={{ padding: 'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)' }}>
        <Notice status="error" isDismissible={false}>
          Failed to load agents: {error}
        </Notice>
      </main>
    );
  }
  if (personas === null) {
    return (
      <main style={{ padding: 'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)' }}>
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Loading agents…</Text>
        </Stack>
      </main>
    );
  }

  const runningCount = personas.filter((p) => metaFor(p.persona).status === 'running').length;

  return (
    <main
      style={{
        maxWidth: 1300,
        margin: '0 auto',
        padding: 'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
      }}
    >
      <Stack direction="column" gap="xs" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
        <Text variant="heading-2xl" render={<h1 />}>
          Agents
        </Text>
        <span className="wa-eyebrow">
          Fleet roster · {personas.length} personas · {runningCount} currently running
        </span>
      </Stack>

      <Card.Root>
        <div className="wa-roster__toolbar">
          <label className="wa-topbar__search" style={{ width: 280 }}>
            <Icon icon={search} size={18} />
            <input type="search" placeholder="Search agents" aria-label="Search agents" />
          </label>
          <Stack direction="row" gap="xs" align="center">
            <button type="button" className="wa-icon-btn" aria-label="Filter">
              <Icon icon={filter} size={18} />
            </button>
            <button type="button" className="wa-icon-btn" aria-label="Change view">
              <Icon icon={grid} size={18} />
            </button>
          </Stack>
        </div>

        <table className="wa-roster">
          <thead>
            <tr>
              <th>Persona</th>
              <th>Mandate</th>
              <th>Model</th>
              <th>Status</th>
              <th>Last run</th>
              <th>Enabled</th>
              <th aria-label="Actions" />
            </tr>
          </thead>
          <tbody>
            {personas.map((p) => {
              const key = personaKeyFrom(p.persona);
              const meta = metaFor(p.persona);
              const enabled = enabledMap[p.persona] ?? p.enabled;
              return (
                <tr
                  key={p.persona}
                  className="wa-roster__row"
                  onClick={() => setEditing(p)}
                >
                  <td>
                    <Stack direction="row" gap="sm" align="center">
                      <PersonaAvatar persona={key} size="md" />
                      <Stack direction="column" gap="xs">
                        <Text
                          variant="body-sm"
                          style={{
                            fontWeight: 'var(--wpds-typography-font-weight-medium)',
                          }}
                        >
                          {displayName(p)}
                        </Text>
                        <span
                          style={{
                            fontSize: 'var(--wpds-typography-font-size-xs)',
                            color: 'var(--wpds-color-fg-content-neutral-weak)',
                          }}
                        >
                          {p.persona}
                        </span>
                      </Stack>
                    </Stack>
                  </td>
                  <td>
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
                    >
                      {meta.mandate}
                    </Text>
                  </td>
                  <td>
                    <button
                      type="button"
                      className="wa-roster__model"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <span className="wa-mono">
                        {p.model_preference ?? 'claude-sonnet-4.6'}
                      </span>
                      <Icon icon={chevronDown} size={14} />
                    </button>
                  </td>
                  <td>
                    {meta.status === 'running' ? (
                      <span className="wa-status wa-status--running">
                        <span className="wa-status-dot" aria-hidden="true" />
                        running
                      </span>
                    ) : (
                      <span className="wa-status wa-status--idle">idle</span>
                    )}
                  </td>
                  <td>
                    <span
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-sm)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {meta.lastRun}
                    </span>
                  </td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <FormToggle
                      checked={enabled}
                      onChange={() =>
                        setEnabledMap((m) => ({ ...m, [p.persona]: !enabled }))
                      }
                    />
                  </td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <button
                      type="button"
                      className="wa-icon-btn"
                      aria-label={`Open ${displayName(p)} actions`}
                    >
                      <Icon icon={moreVertical} size={18} />
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </Card.Root>

      {editing && (
        <EditPersonaModal
          persona={editing}
          mandate={metaFor(editing.persona).mandate}
          systemPrompt={metaFor(editing.persona).systemPrompt}
          onClose={() => setEditing(null)}
        />
      )}
    </main>
  );
}
