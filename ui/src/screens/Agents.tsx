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
  Spinner,
} from '@wordpress/components';
import { plus } from '@wordpress/icons';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import { useNavigate } from 'react-router-dom';
import { ApiError, api, type Connection, type Persona } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import ActionSnackbar from '../components/ActionSnackbar';
import { useAskAgentContext } from '../lib/askAgent';
import { personaToVisible } from '../lib/visibleItems';
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
// implied live run-state the daemon doesn't track. Whether a row is
// operable (full controls) vs. "Coming soon" comes from the daemon's
// `implemented` / `addable` flags on each Persona (see `isOperable` below).
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
    mandate: 'Drafts warm customer notes on recent orders.',
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

// isOperable: row should render with full controls (model dropdown,
// toggle, Run now). A persona is operable when the daemon reports it's
// implemented AND either it's not addable (default-on personas like
// Marketing) or it has been opted in (enabled=true). Addable + disabled =
// "Coming soon" treatment until the operator clicks Add agent on it.
function isOperable(p: Persona): boolean {
  if (!p.implemented) return false;
  if (p.addable && !p.enabled) return false;
  return true;
}

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
        implemented: false,
        addable: false,
      } as Persona),
  );
}

// Sentence case for all display names. Acronyms (SEO) stay uppercased.
// The 'reporting' override exists alongside the Go-side rename in
// Reporting.DisplayName() because operators who added Reporting before
// the rename still have "Reporting agent" cached in their agents.name
// column — this override ensures they see the new name on next page load
// without a DB migration.
function displayName(p: Persona): string {
  if (p.persona === 'marketing') return 'Marketing & SEO';
  if (p.persona === 'inventory') return 'Inventory manager';
  if (p.persona === 'sales-support') return 'Sales support';
  if (p.persona === 'reporting') return 'Reporting';
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

interface StatusCellProps {
  persona: Persona;
  busy: boolean;
  onToggle: () => void;
}

function StatusCell({ persona, busy, onToggle }: StatusCellProps) {
  // Unimplemented personas: the daemon either hasn't seeded them or has no
  // ability handlers wired up. Surface that plainly instead of pretending
  // they can be enabled.
  if (!isOperable(persona)) {
    return <Badge intent="draft">Coming soon</Badge>;
  }
  return (
    <FormToggle
      checked={persona.enabled}
      disabled={busy}
      onChange={onToggle}
    />
  );
}

// Click-stopper for cells that own their own click semantics. DataViews makes
// the row clickable via `onClickItem`; without this wrapper, clicking the
// FormToggle would also fire the row's edit action.
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
  fields: ['mandate', 'enabled', 'run_now'],
  // No default sort — `ALL_PERSONA_KEYS` orders implemented personas first
  // and the "Coming soon" group last; an asc sort by displayName would
  // interleave them ("Accounting" lands above "Marketing & SEO").
  // Comfortable density gives the breathing-room rhythm shown in the
  // WPDS Payouts reference: ~64–72px row height, hairline dividers
  // between rows, vertically-centered cell content. Default ('balanced')
  // crammed the rows; 'compact' is tighter still.
  layout: {
    density: 'comfortable',
  },
};

export default function Agents({ connection, onAskAgent, onChanged }: Props) {
  const navigate = useNavigate();
  const [personas, setPersonas] = useState<Persona[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [view, setView] = useState<View>(DEFAULT_VIEW);
  // Surfaced after a manual-trigger run lands a succeeded status. The
  // "View board" action navigates to /. Auto-dismisses via the WPDS
  // Snackbar default timeout; operator can also dismiss manually.
  const [toast, setToast] = useState<{ text: string } | null>(null);

  useAskAgentContext(
    () => ({
      page: 'agents',
      visible_items: (personas ?? []).map(personaToVisible),
    }),
    [personas],
  );

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

  // Run-now state: tracks which persona is in flight (busy) and the
  // per-persona outcome surfaced after a non-success terminal status. Skips
  // (daemon said "won't seed right now") are an info-level outcome, not an
  // error; failures (request threw, run.status=failed) are error-level.
  // Carries the runId so the inline Cancel-run button has a target.
  const [runBusy, setRunBusy] = useState<{
    persona: string;
    runId: string;
  } | null>(null);
  const [cancelBusy, setCancelBusy] = useState<string | null>(null);
  const [runOutcomes, setRunOutcomes] = useState<
    Record<string, { message: string; isSkip: boolean }>
  >({});

  // Patch state: tracks which row's enabled toggle is currently flipping,
  // plus a per-slug error string surfaced as a Notice above the table.
  // Model edits no longer happen inline (Edit Agent owns that now), so the
  // only patch this screen issues is the enabled flip from StatusCell.
  const [patchBusy, setPatchBusy] = useState<Record<string, true>>({});
  const [patchErrors, setPatchErrors] = useState<Record<string, string>>({});

  const handleToggleEnabled = useCallback(
    async (persona: Persona) => {
      setPatchBusy((prev) => ({ ...prev, [persona.persona]: true }));
      setPatchErrors((prev) => {
        const next = { ...prev };
        delete next[persona.persona];
        return next;
      });
      try {
        const updated = await api.patchAgent(connection, persona.persona, {
          enabled: !persona.enabled,
        });
        setPersonas((prev) =>
          prev
            ? prev.map((p) => (p.persona === updated.persona ? updated : p))
            : prev,
        );
        // Toggling enabled (especially for an addable persona) can shift
        // the row between operable and "Coming soon"; ask App to refresh
        // downstream surfaces (sidebar counts, board) too.
        onChanged?.();
      } catch (e) {
        const msg =
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e);
        setPatchErrors((prev) => ({ ...prev, [persona.persona]: msg }));
      } finally {
        setPatchBusy((prev) => {
          const next = { ...prev };
          delete next[persona.persona];
          return next;
        });
      }
    },
    [connection, onChanged],
  );

  // Stay on /agents through the full run lifecycle. The button's `isBusy`
  // state (driven by runBusy === persona.persona) gives the operator the
  // same loading affordance as Approve in IssueDetail — same context,
  // visible progress, no surprise navigation. Once the run hits a
  // terminal status, clear busy and fire onChanged so App refreshes the
  // board side without the operator leaving this page.
  const handleRunNow = useCallback(
    async (persona: Persona) => {
      if (!isOperable(persona)) return;
      setRunOutcomes((prev) => {
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
        // Request couldn't even land — that's a real error, not a skip.
        setRunOutcomes((prev) => ({
          ...prev,
          [persona.persona]: { message: msg, isSkip: false },
        }));
        setRunBusy(null);
        return;
      }
      setRunBusy({ persona: persona.persona, runId });

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
          // proposal lands on the board. On skip the daemon told us why
          // the run won't produce a proposal (e.g. open-work threshold) —
          // that's an info-level outcome, not an error. On failure
          // (failed / failed_permanent), it's a real error.
          if (status === 'succeeded') {
            onChanged?.();
            setToast({ text: 'Proposal created' });
          } else if (status === 'skipped') {
            const reason = res.run.skip_reason || 'Run skipped';
            setRunOutcomes((prev) => ({
              ...prev,
              [persona.persona]: { message: reason, isSkip: true },
            }));
          } else {
            const reason =
              res.run.failure_reason ||
              res.run.skip_reason ||
              `Run ${status}`;
            setRunOutcomes((prev) => ({
              ...prev,
              [persona.persona]: { message: reason, isSkip: false },
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

  // Cancels the in-flight run for a persona. The endpoint marks the row
  // failed_permanent; the polling loop in handleRunNow detects that
  // terminal status on its next tick (≤2s) and surfaces the cancel reason
  // as an outcome — no separate cleanup path needed here.
  const handleCancelRun = useCallback(
    async (personaSlug: string, runId: string) => {
      setCancelBusy(personaSlug);
      try {
        await api.runs.cancel(connection, runId);
      } catch (e) {
        const msg =
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e);
        setRunOutcomes((prev) => ({
          ...prev,
          [personaSlug]: { message: `Cancel failed: ${msg}`, isSkip: false },
        }));
      } finally {
        setCancelBusy(null);
      }
    },
    [connection],
  );

  const fields = useMemo<Field<Persona>[]>(
    () => [
      {
        id: 'persona',
        label: 'Agent',
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
        id: 'enabled',
        label: 'Status',
        enableSorting: false,
        getValue: ({ item }) =>
          isOperable(item) ? Boolean(item.enabled) : false,
        render: ({ item }) => (
          <NoRowClick>
            <StatusCell
              persona={item}
              busy={!!patchBusy[item.persona]}
              onToggle={() => void handleToggleEnabled(item)}
            />
          </NoRowClick>
        ),
      },
      {
        id: 'run_now',
        label: 'Run',
        enableSorting: false,
        getValue: () => '',
        render: ({ item }) => {
          if (!isOperable(item)) return null;
          const inFlight =
            runBusy?.persona === item.persona ? runBusy.runId : null;
          const isCancelling = cancelBusy === item.persona;
          if (inFlight) {
            return (
              <NoRowClick>
                <Button
                  variant="secondary"
                  __next40pxDefaultSize
                  isDestructive
                  disabled={isCancelling}
                  isBusy={isCancelling}
                  onClick={() => void handleCancelRun(item.persona, inFlight)}
                >
                  <Stack direction="row" gap="xs" align="center">
                    {/* CUSTOM: @wordpress/components Spinner ships with a
                        legacy admin-bar margin (5px 11px 0 0) that pushes
                        it low + adds dead space inside the button. Zero
                        it out so Stack's align="center" + gap="xs" govern
                        the layout. Tracked: DESIGN.md anti-rolls. */}
                    <Spinner style={{ margin: 0 }} />
                    <span>Cancel run</span>
                  </Stack>
                </Button>
              </NoRowClick>
            );
          }
          return (
            <NoRowClick>
              <Button
                variant="secondary"
                __next40pxDefaultSize
                onClick={() => void handleRunNow(item)}
              >
                Run now
              </Button>
            </NoRowClick>
          );
        },
      },
    ],
    [
      runBusy,
      cancelBusy,
      handleRunNow,
      handleCancelRun,
      patchBusy,
      handleToggleEnabled,
    ],
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
        label: 'Edit agent',
        isEligible: (item) => isOperable(item),
        callback: (items) => {
          const p = items[0];
          if (p) navigate(`/agents/${encodeURIComponent(p.persona)}/edit`);
        },
      },
      {
        id: 'view-issues',
        label: 'View issues',
        callback: (items) => {
          const p = items[0];
          if (p) navigate(`/needs-review?persona=${encodeURIComponent(p.persona)}`);
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

  const activeCount = fullRoster.filter((p) => isOperable(p) && p.enabled).length;
  const comingSoonCount = fullRoster.filter((p) => !isOperable(p)).length;

  // Aggregate per-persona outcomes (Run now skips/failures + PATCH failures)
  // into a list of Notices above the table. Surfacing them inline under
  // each control widened the column and broke row alignment; lifting them
  // here keeps the row controls fixed-width and gives each notice room to
  // breathe with its own dismiss. Intent differentiates Run-now *skips*
  // (info — daemon said "won't seed right now") from real errors.
  const errorNotices = useMemo(() => {
    const out: Array<{
      key: string;
      slug: string;
      kind: 'run' | 'patch';
      personaName: string;
      message: string;
      intent: 'info' | 'error';
    }> = [];
    const nameFor = (slug: string) => {
      const p = fullRoster.find((x) => x.persona === slug);
      return p ? displayName(p) : slug;
    };
    for (const [slug, outcome] of Object.entries(runOutcomes)) {
      out.push({
        key: `run:${slug}`,
        slug,
        kind: 'run',
        personaName: nameFor(slug),
        message: outcome.message,
        intent: outcome.isSkip ? 'info' : 'error',
      });
    }
    for (const [slug, msg] of Object.entries(patchErrors)) {
      out.push({
        key: `patch:${slug}`,
        slug,
        kind: 'patch',
        personaName: nameFor(slug),
        message: msg,
        intent: 'error',
      });
    }
    return out;
  }, [runOutcomes, patchErrors, fullRoster]);

  const dismissError = useCallback((kind: 'run' | 'patch', slug: string) => {
    if (kind === 'run') {
      setRunOutcomes((prev) => {
        const next = { ...prev };
        delete next[slug];
        return next;
      });
    } else {
      setPatchErrors((prev) => {
        const next = { ...prev };
        delete next[slug];
        return next;
      });
    }
  }, []);

  // Subtitle is honest about what the daemon actually knows: how many
  // implemented personas are enabled, and how many are still coming soon.
  // When personas are null we still display the canonical fleet size (7);
  // active / coming-soon counts only show once the daemon snapshot loads.
  const subTitle =
    personas === null
      ? 'Fleet roster · 7 agents'
      : `Fleet roster · ${activeCount} active · ${comingSoonCount} coming soon`;

  return (
    <Page
      title="Agents"
      subTitle={subTitle}
      hasPadding
      actions={
        <Stack direction="row" align="center" gap="md">
          <Button
            variant="primary"
            icon={plus}
            __next40pxDefaultSize
            onClick={() => navigate('/agents/add')}
          >
            Add agent
          </Button>
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
          {errorNotices.length > 0 && (
            <Stack
              direction="column"
              gap="sm"
              style={{ marginBottom: 'var(--wpds-dimension-padding-md)' }}
            >
              {errorNotices.map(
                ({ key, slug, kind, personaName, message, intent }) => (
                  <Notice.Root key={key} intent={intent}>
                    <Notice.Description>
                      {kind === 'run'
                        ? intent === 'info'
                          ? `${personaName} skipped this run: ${message}`
                          : `${personaName} run didn't land: ${message}`
                        : `Couldn't update ${personaName}: ${message}`}
                    </Notice.Description>
                    <Notice.CloseIcon
                      label="Dismiss notice"
                      onClick={() => dismissError(kind, slug)}
                    />
                  </Notice.Root>
                ),
              )}
            </Stack>
          )}
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
              if (isOperable(p)) {
                navigate(`/agents/${encodeURIComponent(p.persona)}/edit`);
              }
            }}
            empty={<EmptyState />}
          />
        </>
      )}
      {toast && (
        <ActionSnackbar
          text={toast.text}
          onRemove={() => setToast(null)}
          action={{
            label: 'View board',
            onClick: () => {
              setToast(null);
              navigate('/');
            },
          }}
        />
      )}
    </Page>
  );
}
