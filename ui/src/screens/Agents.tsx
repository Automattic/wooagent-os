import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { Badge, Notice, Stack, Text } from '@wordpress/ui';
import {
  Button,
  FormToggle,
  SelectControl,
  Snackbar,
  Spinner,
  Tooltip,
} from '@wordpress/components';
import { plus } from '@wordpress/icons';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import { useNavigate } from 'react-router-dom';
import { ApiError, api, type Connection, type Persona } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import EditPersonaModal from '../components/EditPersonaModal';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection;
  onAskAgent: () => void;
  /** Fired when a manual run lands a terminal-succeeded status, so App
   *  can refresh the kanban issues without the user navigating there. */
  onChanged?: () => void;
}

// UI-side persona descriptors. `mandate` and `systemPrompt` are design copy
// describing each agent's role (the daemon doesn't store them yet); they're
// kept here as part of the screen's content, not as state. The previous
// `status` ("running"/"idle") and `lastRun` fields were dropped — those
// implied live run-state the daemon doesn't track. See `IMPLEMENTED_PERSONAS`
// below for which agents are actually operational vs. coming soon.
interface PersonaMeta {
  mandate: string;
  systemPrompt: string;
}

const PERSONA_META: Record<string, PersonaMeta> = {
  marketing: {
    mandate: 'Grows organic traffic and on-site conversion.',
    systemPrompt:
      "You are the Marketing & SEO agent for mystore.com. Your primary goal is to grow organic traffic and improve conversion rates. Monitor keyword rankings, suggest meta description updates, and generate product copy that matches the store's warm, approachable brand voice. Always flag changes before writing to WooCommerce.",
  },
  pricing: {
    mandate: 'Protects margin and monitors competitor pricing.',
    systemPrompt:
      'You are the Pricing agent. Watch margins, sales velocity, and competitor signals to propose price moves. Always require human approval before changing live prices.',
  },
  inventory: {
    mandate: 'Keeps stock levels healthy; drafts POs.',
    systemPrompt:
      'You are the Inventory agent. Surface low-stock and overstock issues, propose reorder quantities, and draft purchase orders against approved suppliers.',
  },
  accounting: {
    mandate: 'Reconciles payouts, tracks tax, preps books.',
    systemPrompt:
      'You are the Accounting agent. Reconcile WooPayments and Stripe payouts against the bank, flag tax-relevant changes, and draft month-end summaries.',
  },
  reporting: {
    mandate: 'Produces weekly and monthly digests.',
    systemPrompt:
      'You are the Reporting agent. Generate weekly and monthly digests covering revenue, conversion, and operational anomalies. Investigate ad-hoc questions on request.',
  },
  'sales-support': {
    mandate: 'Drafts replies to pre-sale and order inquiries.',
    systemPrompt:
      'You are the Sales Support agent. Draft customer replies, handle refund triage, and escalate edge cases. Never reply directly without human approval.',
  },
  chief: {
    mandate: 'Triages, routes, and summarizes across the fleet.',
    systemPrompt:
      'You are the Chief of Staff. Triage incoming work, route it to the right specialist agent, and keep the operator briefed on what the fleet is doing.',
  },
};

function metaFor(personaKey: string): PersonaMeta {
  return PERSONA_META[personaKey] ?? { mandate: '—', systemPrompt: '' };
}

// Canonical 7-agent fleet. The daemon may only return a subset; the Roster
// always renders all seven so operators see the full team. Order is:
//   1. Implemented specialists (active personas) first
//   2. Unimplemented specialists ("Coming soon") next
//   3. Chief of Staff ALWAYS last — it's a meta-agent that routes across the
//      fleet, conceptually separate from the specialists. Keep it pinned to
//      the bottom even after it's implemented.
// DEFAULT_VIEW intentionally omits a sort field so this order survives into
// the table.
const ALL_PERSONA_KEYS = [
  'marketing',
  'pricing',
  'sales-support',
  'inventory',
  'accounting',
  'reporting',
  'chief',
];

// Personas the daemon has actually seeded and wired to ability handlers.
// Anything outside this set renders as "Coming soon" — the row still appears
// in the roster, but its controls (enable toggle, model preference, edit)
// are inert until the daemon learns to drive it. Keep in sync with the
// `defaultPersonas` list in daemon/internal/cli/init.go.
const IMPLEMENTED_PERSONAS = new Set<string>([
  'marketing',
  'pricing',
  'sales-support',
]);

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
  // Plain flex div instead of Stack — Stack's gap is set via internal CSS
  // that resists inline overrides, and we need an exact 10px gap (between
  // WPDS gap-sm 8px and gap-md 16px). Slug below the name was removed —
  // the avatar's two-letter monogram already encodes the slug visually.
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <PersonaAvatar persona={personaKeyFrom(persona.persona)} size="md" />
      <Text
        variant="body-sm"
        style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
      >
        {displayName(persona)}
      </Text>
    </div>
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
  // Read-only until daemon supports model PATCH. Shows the daemon's current
  // preference (or the default) without an interactive control that can't
  // persist its changes. Wire to a real PATCH endpoint and re-enable when
  // daemon support lands.
  const current = persona.model_preference ?? 'anthropic/claude-sonnet-4-6';
  // Wrapper width keeps the column at ~180px regardless of `table-layout`
  // hints. DataViews's per-column `view.layout.styles.model.width` is set too,
  // but auto-layout treats it as a preference; sizing the content itself is
  // the only reliable lever.
  return (
    <div style={{ minWidth: 180 }}>
      <Tooltip text="Model preference is read-only until operator controls land.">
        {/* CUSTOM: span wrapper so Tooltip has a non-disabled hover/focus
            anchor — disabled SelectControl drops pointer events. Same pattern
            used by the Add agent button in this screen's actions slot. */}
        <span style={{ display: 'inline-flex', width: '100%' }} tabIndex={0}>
          <SelectControl
            __nextHasNoMarginBottom
            label="Model"
            hideLabelFromVision
            value={current}
            options={modelOptionsFor(current)}
            disabled
            aria-disabled="true"
            onChange={() => {}}
          />
        </span>
      </Tooltip>
    </div>
  );
}

function StatusCell({ persona }: { persona: Persona }) {
  // Unimplemented personas: the daemon either hasn't seeded them or has no
  // ability handlers wired up. Surface that plainly instead of pretending
  // they can be enabled. Implemented personas show daemon truth via a
  // disabled FormToggle — controls land once /v1/agents accepts PATCH.
  if (!IMPLEMENTED_PERSONAS.has(persona.persona)) {
    return <Badge intent="draft">Coming soon</Badge>;
  }
  return (
    <Tooltip text="Enable / disable lands with daemon operator controls.">
      {/* CUSTOM: span wrapper to give Tooltip a non-disabled hover/focus
          anchor (FormToggle disabled drops pointer events). Same pattern as
          ModelCell and the Add agent button. */}
      <span style={{ display: 'inline-flex' }} tabIndex={0}>
        <FormToggle
          checked={persona.enabled}
          disabled
          aria-disabled="true"
          onChange={() => {}}
        />
      </span>
    </Tooltip>
  );
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
        They'll show up here as soon as WooAgent reports them.
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
  fields: ['mandate', 'model', 'enabled', 'run_now'],
  // No default sort — `ALL_PERSONA_KEYS` orders implemented personas first
  // and the "Coming soon" group last; an asc sort by displayName would
  // interleave them ("Accounting" lands above "Marketing & SEO").
  // Comfortable density gives the breathing-room rhythm shown in the
  // WPDS Payouts reference: ~64–72px row height, hairline dividers
  // between rows, vertically-centered cell content. Default ('balanced')
  // crammed the rows; 'compact' is tighter still.
  layout: {
    density: 'comfortable',
    styles: {
      // Paired with the same minWidth on the ModelCell content wrapper —
      // see ModelCell. 180px fits short labels like "GPT-5" and the
      // recommended Claude variants without dominating the row.
      model: { width: '180px' },
    },
  },
};

export default function Agents({ connection, onAskAgent, onChanged }: Props) {
  const navigate = useNavigate();
  const [personas, setPersonas] = useState<Persona[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<Persona | null>(null);
  const [view, setView] = useState<View>(DEFAULT_VIEW);
  // Surfaced after a manual-trigger run lands a succeeded status. The
  // "View board" action navigates to /. Auto-dismisses via the WPDS
  // Snackbar default timeout; operator can also dismiss manually.
  const [toast, setToast] = useState<{ text: string } | null>(null);

  const fetchAgents = useCallback(
    async (signal: { cancelled: boolean }) => {
      setError(null);
      try {
        const res = await api.agents(connection);
        if (signal.cancelled) return;
        setPersonas(res.agents);
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

  // Run-now state: tracks which persona is being triggered (busy) and
  // per-persona error messages (if triggerRun throws).
  const [runBusy, setRunBusy] = useState<string | null>(null);
  const [runErrors, setRunErrors] = useState<Record<string, string>>({});

  // Stay on /agents through the full run lifecycle. The button's `isBusy`
  // state (driven by runBusy === persona.persona) gives the operator the
  // same loading affordance as Approve in IssueDetail — same context,
  // visible progress, no surprise navigation. Once the run hits a
  // terminal status, clear busy and fire onChanged so App refreshes the
  // board side without the operator leaving this page.
  const handleRunNow = useCallback(
    async (persona: Persona) => {
      if (!IMPLEMENTED_PERSONAS.has(persona.persona)) return;
      setRunBusy(persona.persona);
      setRunErrors((prev) => {
        const next = { ...prev };
        delete next[persona.persona];
        return next;
      });

      let runId: string;
      try {
        const { run } = await api.runs.trigger(connection, persona.persona);
        runId = run.id;
      } catch (e) {
        const msg =
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e);
        setRunErrors((prev) => ({ ...prev, [persona.persona]: msg }));
        setRunBusy(null);
        return;
      }

      // Poll the run until terminal. Pricing routinely takes 30-60s
      // (web_search across retailers); a 2s cadence keeps the perceived
      // progress lively without hammering the daemon.
      const stopPolling = (clearBusy: boolean) => {
        if (clearBusy) setRunBusy(null);
      };
      let cancelled = false;
      const tick = async () => {
        if (cancelled) return;
        try {
          const res = await api.runs.get(connection, runId);
          const status = res.run.status;
          if (status === 'queued' || status === 'running') {
            setTimeout(() => void tick(), 2_000);
            return;
          }
          // Terminal. On success, ask App to refresh issues so any new
          // proposal lands on the board. On skip/fail, surface a short
          // hint inline so the operator knows why the spinner stopped.
          if (status === 'succeeded') {
            onChanged?.();
            setToast({ text: 'Proposal created' });
          } else {
            const reason =
              res.run.skip_reason ||
              res.run.failure_reason ||
              `Run ${status}`;
            setRunErrors((prev) => ({
              ...prev,
              [persona.persona]: reason,
            }));
          }
          stopPolling(true);
        } catch (e) {
          // Transient fetch error — retry once after the normal interval.
          // If it keeps failing, the operator can refresh.
          if (!cancelled) setTimeout(() => void tick(), 2_000);
        }
      };
      void tick();

      // Note: there's no cleanup if the user navigates away mid-run.
      // The polling cancels via `cancelled` only if we surface it; for
      // a simple keep-on-page flow the worst case is a few orphaned
      // fetches after navigation, which the daemon ignores.
    },
    [connection, onChanged],
  );

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
        id: 'enabled',
        label: 'Status',
        enableSorting: false,
        getValue: ({ item }) =>
          IMPLEMENTED_PERSONAS.has(item.persona) ? Boolean(item.enabled) : false,
        render: ({ item }) => (
          <NoRowClick>
            <StatusCell persona={item} />
          </NoRowClick>
        ),
      },
      {
        id: 'run_now',
        label: 'Run',
        enableSorting: false,
        getValue: () => '',
        render: ({ item }) => {
          if (!IMPLEMENTED_PERSONAS.has(item.persona)) return null;
          const isBusy = runBusy === item.persona;
          const err = runErrors[item.persona];
          return (
            <NoRowClick>
              <Stack direction="column" gap="xs">
                <Button
                  variant="secondary"
                  __next40pxDefaultSize
                  isBusy={isBusy}
                  disabled={isBusy}
                  onClick={() => void handleRunNow(item)}
                >
                  Run now
                </Button>
                {err && (
                  <Text
                    variant="body-sm"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-warning)',
                      maxWidth: 160,
                    }}
                  >
                    {err}
                  </Text>
                )}
              </Stack>
            </NoRowClick>
          );
        },
      },
    ],
    [runBusy, runErrors, handleRunNow],
  );

  // Row click drives the edit flow via `onClickItem`. The actions are also
  // exposed under the per-row ⋮ menu — no `isPrimary` flag on either, so
  // DataViews keeps them in the secondary-actions dropdown rather than
  // rendering them inline as text buttons. `supportsBulk` is omitted
  // everywhere so DataViews does not render a selection column. Edit is
  // gated to implemented personas — the unimplemented rows have no persona
  // to configure yet.
  const actions = useMemo<Action<Persona>[]>(
    () => [
      {
        id: 'edit',
        label: 'Edit persona',
        isEligible: (item) => IMPLEMENTED_PERSONAS.has(item.persona),
        callback: (items) => {
          const p = items[0];
          if (p) setEditing(p);
        },
      },
      {
        id: 'view-issues',
        label: 'View issues',
        callback: (items) => {
          const p = items[0];
          if (p) navigate(`/?persona=${encodeURIComponent(p.persona)}`);
        },
      },
    ],
    [navigate],
  );

  const fullRoster = useMemo(
    () => (personas ? buildFullRoster(personas) : []),
    [personas],
  );

  const { data: shaped, paginationInfo } = useMemo(
    () => filterSortAndPaginate(fullRoster, view, fields),
    [fullRoster, view, fields],
  );

  const activeCount = fullRoster.filter(
    (p) => IMPLEMENTED_PERSONAS.has(p.persona) && p.enabled,
  ).length;
  const comingSoonCount = fullRoster.filter(
    (p) => !IMPLEMENTED_PERSONAS.has(p.persona),
  ).length;

  // Subtitle is honest about what the daemon actually knows: how many
  // implemented personas are enabled, and how many are still coming soon.
  // When personas are null we still display the canonical fleet size (7);
  // active / coming-soon counts only show once the daemon snapshot loads.
  const subTitle =
    personas === null
      ? 'Fleet roster · 7 personas'
      : `Fleet roster · ${activeCount} active · ${comingSoonCount} coming soon`;

  return (
    <Page
      title="Agents"
      subTitle={subTitle}
      actions={
        <Stack direction="row" align="center" gap="md">
          <Tooltip text="Out of scope for phase 1">
            {/* CUSTOM: span wrapper around the disabled button. (a) WPDS
                Tooltip can't fire on a `<button disabled>` because the
                browser drops pointer events on disabled buttons. (b) The
                span gives Ariakit a non-disabled anchor for hover/focus
                while the inner Button keeps its native disabled styling.
                (c) Pattern recommended by the WPDS Storybook tooltip
                examples; follow-up if WPDS ships a first-class
                disabled-tooltip wrapper. */}
            <span style={{ display: 'inline-flex' }} tabIndex={0}>
              <Button
                variant="primary"
                icon={plus}
                __next40pxDefaultSize
                disabled
                aria-disabled="true"
              >
                Add agent
              </Button>
            </span>
          </Tooltip>
          <PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />
        </Stack>
      }
    >
      {error ? (
        <Notice.Root intent="error">
          <Notice.Description>
            Hmm, couldn't load your agents right now. ({error})
          </Notice.Description>
          <Notice.Actions>
            <Notice.ActionButton onClick={handleRetry}>
              Retry
            </Notice.ActionButton>
          </Notice.Actions>
        </Notice.Root>
      ) : personas === null ? (
        <Stack direction="row" gap="sm" align="center">
          <Spinner />{' '}
          <Text variant="body-sm">Getting your agents ready…</Text>
        </Stack>
      ) : (
        <>
          <DataViews<Persona>
            view={view}
            onChangeView={setView}
            fields={fields}
            actions={actions}
            data={shaped}
            getItemId={(p) => p.persona}
            paginationInfo={paginationInfo}
            defaultLayouts={{ table: {} }}
            onClickItem={(p) => {
              if (IMPLEMENTED_PERSONAS.has(p.persona)) setEditing(p);
            }}
            empty={<EmptyState />}
          />

          {editing && (
            <EditPersonaModal
              persona={editing}
              mandate={metaFor(editing.persona).mandate}
              systemPrompt={metaFor(editing.persona).systemPrompt}
              onClose={() => setEditing(null)}
            />
          )}
        </>
      )}
      {toast && (
        <div className="wa-snackbar-host">
          <Snackbar
            onRemove={() => setToast(null)}
            actions={[
              {
                label: 'View board',
                onClick: () => {
                  setToast(null);
                  navigate('/');
                },
              },
            ]}
          >
            {toast.text}
          </Snackbar>
        </div>
      )}
    </Page>
  );
}
