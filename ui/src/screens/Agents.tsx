import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import {
  FormToggle,
  Notice,
  SelectControl,
  Spinner,
} from '@wordpress/components';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
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

// Canonical 7-agent fleet. The daemon may only return a subset; the Roster
// always renders all seven so operators see the full team.
const ALL_PERSONA_KEYS = [
  'marketing',
  'pricing',
  'inventory',
  'accounting',
  'reporting',
  'sales-support',
  'chief',
];

function buildFullRoster(daemonPersonas: Persona[]): Persona[] {
  const byKey = new Map(daemonPersonas.map((p) => [p.persona, p]));
  return ALL_PERSONA_KEYS.map(
    (key) =>
      byKey.get(key) ??
      ({
        persona: key,
        name: key,
        enabled: false,
        model_preference: undefined,
      } as Persona),
  );
}

// V1 model options. Hardcoded until the daemon exposes the available-models
// endpoint.
const MODEL_OPTIONS: Array<{ label: string; value: string }> = [
  { label: 'Claude Sonnet 4.6', value: 'anthropic/claude-sonnet-4-6' },
  { label: 'Claude Opus 4.7', value: 'anthropic/claude-opus-4-7' },
  { label: 'Claude Haiku 4.5', value: 'anthropic/claude-haiku-4-5' },
  { label: 'Gemini 2.5 Pro', value: 'google/gemini-2.5-pro' },
  { label: 'GPT-5', value: 'openai/gpt-5' },
];

function modelOptionsFor(current: string | undefined) {
  if (current && !MODEL_OPTIONS.some((o) => o.value === current)) {
    return [{ label: current, value: current }, ...MODEL_OPTIONS];
  }
  return MODEL_OPTIONS;
}

// Sentence case for all display names. Acronyms (SEO) stay uppercased.
function displayName(p: Persona): string {
  if (p.persona === 'marketing') return 'Marketing & SEO';
  if (p.persona === 'inventory') return 'Inventory manager';
  if (p.persona === 'sales-support') return 'Sales support';
  if (p.persona === 'chief') return 'Chief of staff';
  const fallback = p.name || p.persona;
  return fallback.charAt(0).toUpperCase() + fallback.slice(1).toLowerCase();
}

// === Cells === //

function PersonaCell({ persona }: { persona: Persona }) {
  return (
    <Stack direction="row" gap="sm" align="center">
      <PersonaAvatar persona={personaKeyFrom(persona.persona)} size="md" />
      <Stack direction="column" gap="xs">
        <Text
          variant="body-sm"
          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
        >
          {displayName(persona)}
        </Text>
        <span
          style={{
            fontSize: 'var(--wpds-typography-font-size-xs)',
            color: 'var(--wpds-color-fg-content-neutral-weak)',
          }}
        >
          {persona.persona}
        </span>
      </Stack>
    </Stack>
  );
}

function MandateCell({ persona }: { persona: Persona }) {
  return (
    <Text
      variant="body-sm"
      style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
    >
      {metaFor(persona.persona).mandate}
    </Text>
  );
}

function ModelCell({ persona }: { persona: Persona }) {
  // Local-only state until daemon supports model PATCH. Pre-populated from the
  // daemon snapshot; changes don't persist yet.
  const initial = persona.model_preference ?? 'anthropic/claude-sonnet-4-6';
  const [model, setModel] = useState<string>(initial);
  return (
    <SelectControl
      __nextHasNoMarginBottom
      label="Model"
      hideLabelFromVision
      value={model}
      options={modelOptionsFor(model)}
      onChange={(next) => setModel(next ?? initial)}
    />
  );
}

function StatusCell({ persona }: { persona: Persona }) {
  const meta = metaFor(persona.persona);
  if (meta.status === 'running') {
    return (
      <span className="wa-status wa-status--running">
        <span className="wa-status-dot" aria-hidden="true" />
        running
      </span>
    );
  }
  return <span className="wa-status wa-status--idle">idle</span>;
}

function LastRunCell({ persona }: { persona: Persona }) {
  return (
    <span
      style={{
        fontSize: 'var(--wpds-typography-font-size-sm)',
        color: 'var(--wpds-color-fg-content-neutral-weak)',
      }}
    >
      {metaFor(persona.persona).lastRun}
    </span>
  );
}

function EnabledCell({
  enabled,
  onToggle,
}: {
  enabled: boolean;
  onToggle: () => void;
}) {
  return <FormToggle checked={enabled} onChange={onToggle} />;
}

// Click-stopper for cells that own their own click semantics. DataViews makes
// the row clickable via `onClickItem`; without this wrapper, clicking the model
// SelectControl or the FormToggle would also fire the row's edit action.
function NoRowClick({ children }: { children: ReactNode }) {
  return (
    <div
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
    >
      {children}
    </div>
  );
}

function EmptyState() {
  return (
    <Stack
      direction="column"
      gap="sm"
      align="center"
      style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}
    >
      <Text variant="body-md">No agents yet.</Text>
      <Text
        variant="body-sm"
        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
      >
        They'll show up here as soon as your daemon reports them.
      </Text>
    </Stack>
  );
}

// === Screen === //

const DEFAULT_VIEW: View = {
  type: 'table',
  search: '',
  page: 1,
  perPage: 25,
  titleField: 'persona',
  fields: ['mandate', 'model', 'status', 'last_run', 'enabled'],
  sort: { field: 'persona', direction: 'asc' },
};

export default function Agents({ connection }: Props) {
  const [personas, setPersonas] = useState<Persona[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<Persona | null>(null);
  // Local-only enabled state — daemon has no PATCH /v1/agents yet.
  const [enabledMap, setEnabledMap] = useState<Record<string, boolean>>({});
  const [view, setView] = useState<View>(DEFAULT_VIEW);

  const fetchAgents = useCallback(
    async (signal: { cancelled: boolean }) => {
      setError(null);
      try {
        const res = await api.agents(connection);
        if (signal.cancelled) return;
        setPersonas(res.agents);
        const map: Record<string, boolean> = {};
        for (const a of res.agents) map[a.persona] = a.enabled;
        setEnabledMap(map);
      } catch (e) {
        if (!signal.cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    },
    [connection],
  );

  useEffect(() => {
    const signal = { cancelled: false };
    void fetchAgents(signal);
    return () => {
      signal.cancelled = true;
    };
  }, [fetchAgents]);

  const handleRetry = () => {
    setPersonas(null);
    const signal = { cancelled: false };
    void fetchAgents(signal);
  };

  // Field config — closures over enabledMap / setEnabledMap are intentional.
  // Memoised so DataViews receives a stable reference between toggle flips.
  const fields = useMemo<Field<Persona>[]>(
    () => [
      {
        id: 'persona',
        label: 'Persona',
        enableHiding: false,
        enableGlobalSearch: true,
        getValue: ({ item }) => displayName(item),
        render: ({ item }) => <PersonaCell persona={item} />,
      },
      {
        id: 'mandate',
        label: 'Mandate',
        enableSorting: false,
        enableGlobalSearch: true,
        getValue: ({ item }) => metaFor(item.persona).mandate,
        render: ({ item }) => <MandateCell persona={item} />,
      },
      {
        id: 'model',
        label: 'Model',
        enableSorting: false,
        getValue: ({ item }) => item.model_preference ?? '',
        render: ({ item }) => (
          <NoRowClick>
            <ModelCell persona={item} />
          </NoRowClick>
        ),
      },
      {
        id: 'status',
        label: 'Status',
        elements: [
          { value: 'running', label: 'Running' },
          { value: 'idle', label: 'Idle' },
        ],
        getValue: ({ item }) => metaFor(item.persona).status,
        render: ({ item }) => <StatusCell persona={item} />,
      },
      {
        id: 'last_run',
        label: 'Last run',
        enableSorting: false,
        getValue: ({ item }) => metaFor(item.persona).lastRun,
        render: ({ item }) => <LastRunCell persona={item} />,
      },
      {
        id: 'enabled',
        label: 'Enabled',
        enableSorting: false,
        getValue: ({ item }) =>
          Boolean(enabledMap[item.persona] ?? item.enabled),
        render: ({ item }) => {
          const enabled = enabledMap[item.persona] ?? item.enabled;
          return (
            <NoRowClick>
              <EnabledCell
                enabled={enabled}
                onToggle={() =>
                  setEnabledMap((m) => ({
                    ...m,
                    [item.persona]: !enabled,
                  }))
                }
              />
            </NoRowClick>
          );
        },
      },
    ],
    [enabledMap],
  );

  // Row click drives the edit flow via `onClickItem`. The action is also
  // exposed under the per-row ⋮ menu — no `isPrimary` flag, so DataViews
  // keeps it in the secondary-actions dropdown rather than rendering it
  // inline as a text button (which made the Actions column read as a
  // labelled button instead of an icon menu). `supportsBulk` is omitted
  // everywhere so DataViews does not render a selection column.
  const actions = useMemo<Action<Persona>[]>(
    () => [
      {
        id: 'edit',
        label: 'Edit persona',
        callback: (items) => {
          const p = items[0];
          if (p) setEditing(p);
        },
      },
    ],
    [],
  );

  const fullRoster = useMemo(
    () => (personas ? buildFullRoster(personas) : []),
    [personas],
  );

  const { data: shaped, paginationInfo } = useMemo(
    () => filterSortAndPaginate(fullRoster, view, fields),
    [fullRoster, view, fields],
  );

  if (error) {
    return (
      <main
        style={{
          padding:
            'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
        }}
      >
        <Notice
          status="error"
          isDismissible={false}
          actions={[
            { label: 'Retry', onClick: handleRetry, variant: 'primary' },
          ]}
        >
          Hmm, couldn't load your agents right now. ({error})
        </Notice>
      </main>
    );
  }
  if (personas === null) {
    return (
      <main
        style={{
          padding:
            'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
        }}
      >
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Getting your agents ready…</Text>
        </Stack>
      </main>
    );
  }

  const runningCount = fullRoster.filter(
    (p) => metaFor(p.persona).status === 'running',
  ).length;

  return (
    <main
      style={{
        maxWidth: 1300,
        margin: '0 auto',
        padding:
          'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
      }}
    >
      <Stack
        direction="column"
        gap="xs"
        style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
      >
        <Text variant="heading-2xl" render={<h1 />}>
          Agents
        </Text>
        <span className="wa-eyebrow">
          Fleet roster · {fullRoster.length} personas · {runningCount}{' '}
          currently running
        </span>
      </Stack>

      <Card.Root>
        <DataViews<Persona>
          view={view}
          onChangeView={setView}
          fields={fields}
          actions={actions}
          data={shaped}
          getItemId={(p) => p.persona}
          paginationInfo={paginationInfo}
          defaultLayouts={{ table: {} }}
          onClickItem={(p) => setEditing(p)}
          empty={<EmptyState />}
        />
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
