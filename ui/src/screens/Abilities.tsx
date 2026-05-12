import {
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { Badge, Notice, Stack, Text } from '@wordpress/ui';
import { Button, Modal, Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import {
  api,
  type Ability,
  type AbilityTrustState,
  type Connection,
} from '../api/client';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection | null;
  onAskAgent: () => void;
}

// Trust-state vocabulary the daemon surfaces. Labels are sentence case per
// DESIGN.md; intents reuse WPDS intents — `stable` for trusted, `medium`
// for the schema-drift case (operator action needed but not catastrophic),
// `informational` for newly-discovered rows.
const TRUST_LABEL: Record<AbilityTrustState, string> = {
  trusted: 'Trusted',
  new: 'New',
  schema_changed: 'Changed',
};

const TRUST_INTENT: Record<
  AbilityTrustState,
  'stable' | 'medium' | 'informational'
> = {
  trusted: 'stable',
  new: 'informational',
  schema_changed: 'medium',
};

// DataViews 'elements' for the trust-state filter dropdown. The `value`
// has to round-trip through the same string the daemon emits.
const TRUST_ELEMENTS: Array<{ value: AbilityTrustState; label: string }> = [
  { value: 'trusted', label: 'Trusted' },
  { value: 'new', label: 'New' },
  { value: 'schema_changed', label: 'Changed' },
];

function relativeTime(iso: string | undefined): string {
  if (!iso) return '—';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '—';
  const seconds = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  // Older than a month — just show the date so the badge doesn't read
  // "120d ago" (which nobody parses faster than a date).
  return new Date(t).toLocaleDateString();
}

// === Cells === //

function NameCell({ ability }: { ability: Ability }) {
  return (
    <Stack direction="column" gap="xs">
      <Text
        variant="body-sm"
        style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
      >
        {ability.title || ability.name}
      </Text>
      {ability.title && ability.title !== ability.name && (
        <Text
          variant="body-sm"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          {ability.name}
        </Text>
      )}
    </Stack>
  );
}

function StoreCell({ ability }: { ability: Ability }) {
  // Strip the protocol so the column reads "mystore.com" instead of
  // "https://mystore.com". Identifiers stay in body font (DESIGN.md).
  const display = (ability.store_url || '').replace(/^https?:\/\//, '');
  return (
    <Text
      variant="body-sm"
      style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
    >
      {display || ability.store_id}
    </Text>
  );
}

function VersionCell({ ability }: { ability: Ability }) {
  return (
    <Text
      variant="body-sm"
      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
    >
      {ability.version || '—'}
    </Text>
  );
}

function TrustCell({ state }: { state: AbilityTrustState }) {
  return <Badge intent={TRUST_INTENT[state]}>{TRUST_LABEL[state]}</Badge>;
}

function LastSeenCell({ ability }: { ability: Ability }) {
  return (
    <span
      style={{
        fontSize: 'var(--wpds-typography-font-size-sm)',
        color: 'var(--wpds-color-fg-content-neutral-weak)',
      }}
    >
      {relativeTime(ability.last_seen_at)}
    </span>
  );
}

// === Inspector modal === //

function InspectorModal({
  ability,
  busy,
  onTrust,
  onClose,
}: {
  ability: Ability;
  busy: boolean;
  onTrust: (id: string) => void;
  onClose: () => void;
}) {
  const trustable =
    ability.trust_state === 'new' || ability.trust_state === 'schema_changed';
  return (
    <Modal title={ability.title || ability.name} onRequestClose={onClose} size="medium">
      <Stack direction="column" gap="md">
        <Stack direction="row" gap="sm" align="center">
          <TrustCell state={ability.trust_state} />
          {ability.version && (
            <Badge intent="none">{`v${ability.version}`}</Badge>
          )}
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            Last seen {relativeTime(ability.last_seen_at)}
          </Text>
        </Stack>

        {ability.description && (
          <Text variant="body-md">{ability.description}</Text>
        )}

        <Stack direction="column" gap="xs">
          <Text
            variant="body-sm"
            style={{
              fontWeight: 'var(--wpds-typography-font-weight-medium)',
              color: 'var(--wpds-color-fg-content-neutral)',
            }}
          >
            Identifier
          </Text>
          <Text variant="body-sm">{ability.name}</Text>
        </Stack>

        {ability.store_url && (
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
                color: 'var(--wpds-color-fg-content-neutral)',
              }}
            >
              Store
            </Text>
            <Text variant="body-sm">
              {ability.store_url.replace(/^https?:\/\//, '')}
            </Text>
          </Stack>
        )}

        {ability.schema && (
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
                color: 'var(--wpds-color-fg-content-neutral)',
              }}
            >
              Schema
            </Text>
            {/* CUSTOM: pre-formatted JSON dump for the schema details panel.
                (a) WPDS doesn't have a JSON viewer / structured schema component
                — DataViews is for tabular data, Card is for grouped content,
                neither preserves whitespace + line breaks.
                (b) Wrapped in a scrollable bordered container; uses the body
                font (no monospace per DESIGN.md) but preserves whitespace via
                white-space: pre-wrap so JSON keys stay on their own lines.
                (c) Documented as the only V1 "raw schema" surface; future
                iteration may replace with a structured params/returns table. */}
            <pre
              style={{
                margin: 0,
                padding: 'var(--wpds-dimension-padding-sm)',
                background: 'var(--wpds-color-bg-surface-neutral-subtle)',
                border:
                  '1px solid var(--wpds-color-stroke-neutral-subtle)',
                borderRadius: 'var(--wpds-border-radius-sm)',
                fontFamily: 'var(--wpds-typography-font-family-body)',
                fontSize: 'var(--wpds-typography-font-size-sm)',
                lineHeight: 'var(--wpds-typography-line-height-sm)',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                maxHeight: 320,
                overflow: 'auto',
              }}
            >
              {JSON.stringify(ability.schema, null, 2)}
            </pre>
          </Stack>
        )}

        <Stack direction="row" gap="sm" justify="flex-end">
          <Button variant="tertiary" __next40pxDefaultSize onClick={onClose} disabled={busy}>
            Close
          </Button>
          {trustable && (
            <Button
              __next40pxDefaultSize
              variant="primary"
              onClick={() => onTrust(ability.id)}
              isBusy={busy}
              disabled={busy}
            >
              {ability.trust_state === 'schema_changed'
                ? 'Re-trust at current schema'
                : 'Trust this ability'}
            </Button>
          )}
        </Stack>
      </Stack>
    </Modal>
  );
}

// === Empty / loading states === //

function EmptyState({ filtered }: { filtered: boolean }) {
  return (
    <Stack
      direction="column"
      gap="sm"
      align="center"
      style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}
    >
      <Text variant="body-md">
        {filtered
          ? 'No abilities match this filter.'
          : 'No abilities yet.'}
      </Text>
      <Text
        variant="body-sm"
        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
      >
        {filtered
          ? 'Try clearing the filter to see the full set.'
          : "They'll appear here once your daemon finishes discovering them from your paired store."}
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
  titleField: 'name',
  fields: ['store', 'version', 'trust_state', 'last_seen'],
  sort: { field: 'name', direction: 'asc' },
  layout: { density: 'comfortable' },
};

export default function Abilities({ connection, onAskAgent }: Props) {
  const [abilities, setAbilities] = useState<Ability[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [view, setView] = useState<View>(DEFAULT_VIEW);
  const [inspecting, setInspecting] = useState<Ability | null>(null);
  const [trustBusy, setTrustBusy] = useState(false);

  const fetchAbilities = useCallback(
    async (signal: { cancelled: boolean }) => {
      if (!connection) {
        setAbilities([]);
        return;
      }
      setError(null);
      try {
        const res = await api.abilities.list(connection);
        if (!signal.cancelled) setAbilities(res.abilities);
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
    void fetchAbilities(signal);
    return () => {
      signal.cancelled = true;
    };
  }, [fetchAbilities]);

  const handleTrust = useCallback(
    async (id: string) => {
      if (!connection) return;
      setTrustBusy(true);
      try {
        const updated = await api.abilities.trust(connection, id);
        setAbilities((prev) =>
          prev ? prev.map((a) => (a.id === id ? updated : a)) : prev,
        );
        setInspecting(updated);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setTrustBusy(false);
      }
    },
    [connection],
  );

  const handleRetry = () => {
    setAbilities(null);
    const signal = { cancelled: false };
    void fetchAbilities(signal);
  };

  const fields = useMemo<Field<Ability>[]>(
    () => [
      {
        id: 'name',
        label: 'Ability',
        enableHiding: false,
        enableGlobalSearch: true,
        getValue: ({ item }) => item.title || item.name,
        render: ({ item }) => <NameCell ability={item} />,
      },
      {
        id: 'store',
        label: 'Store',
        enableSorting: true,
        enableGlobalSearch: true,
        getValue: ({ item }) =>
          (item.store_url || '').replace(/^https?:\/\//, '') || item.store_id,
        render: ({ item }) => <StoreCell ability={item} />,
      },
      {
        id: 'version',
        label: 'Version',
        enableSorting: true,
        getValue: ({ item }) => item.version || '',
        render: ({ item }) => <VersionCell ability={item} />,
      },
      {
        id: 'trust_state',
        label: 'Trust',
        elements: TRUST_ELEMENTS,
        getValue: ({ item }) => item.trust_state,
        render: ({ item }) => <TrustCell state={item.trust_state} />,
      },
      {
        id: 'last_seen',
        label: 'Last seen',
        enableSorting: true,
        getValue: ({ item }) => item.last_seen_at || '',
        render: ({ item }) => <LastSeenCell ability={item} />,
      },
    ],
    [],
  );

  // Per-row "Trust" surfaces under the ⋮ menu and only when actionable.
  // schema_changed and new rows can be trusted; trusted rows are no-ops
  // and we hide the action so DataViews doesn't disable-flicker.
  const actions = useMemo<Action<Ability>[]>(
    () => [
      {
        id: 'trust',
        label: 'Trust this ability',
        isEligible: (ability) =>
          ability.trust_state === 'new' ||
          ability.trust_state === 'schema_changed',
        callback: (items) => {
          const a = items[0];
          if (a) void handleTrust(a.id);
        },
      },
      {
        id: 'inspect',
        label: 'Inspect',
        callback: (items) => {
          const a = items[0];
          if (a) setInspecting(a);
        },
      },
    ],
    [handleTrust],
  );

  const data = abilities ?? [];

  const { data: shaped, paginationInfo } = useMemo(
    () => filterSortAndPaginate(data, view, fields),
    [data, view, fields],
  );

  const counts = useMemo(() => {
    const c = { total: data.length, trusted: 0, needsReview: 0 };
    for (const a of data) {
      if (a.trust_state === 'trusted') c.trusted += 1;
      else c.needsReview += 1;
    }
    return c;
  }, [data]);

  const subTitle =
    abilities === null
      ? 'Tools your agents can call'
      : counts.total === 0
        ? 'Tools your agents can call'
        : `Tools your agents can call · ${counts.total} total · ${counts.trusted} trusted · ${counts.needsReview} need review`;

  const filteredEmpty = Boolean(view.search) || Boolean(view.filters?.length);

  return (
    <Page
      title="Abilities"
      subTitle={subTitle}
      actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
    >
      {error ? (
        <Notice.Root intent="error">
          <Notice.Description>
            Hmm, couldn't load your abilities right now. ({error})
          </Notice.Description>
          <Notice.Actions>
            <Notice.ActionButton onClick={handleRetry}>
              Retry
            </Notice.ActionButton>
          </Notice.Actions>
        </Notice.Root>
      ) : abilities === null ? (
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Reading abilities from your store…</Text>
        </Stack>
      ) : (
        <>
          <DataViews<Ability>
            view={view}
            onChangeView={setView}
            fields={fields}
            actions={actions}
            data={shaped}
            getItemId={(a) => a.id}
            paginationInfo={paginationInfo}
            defaultLayouts={{ table: {} }}
            onClickItem={(a) => setInspecting(a)}
            empty={<EmptyState filtered={filteredEmpty} />}
          />
          {inspecting && (
            <InspectorModal
              ability={inspecting}
              busy={trustBusy}
              onTrust={handleTrust}
              onClose={() => setInspecting(null)}
            />
          )}
        </>
      )}
    </Page>
  );
}
