import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import { api, type Connection, type Issue } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import PageGlobalActions from '../components/PageGlobalActions';
import {
  personaDisplayName,
  personaElementsFrom,
  relativeTime,
} from '../lib/boardItems';
import { useAskAgentContext } from '../lib/askAgent';
import { issueToVisible } from '../lib/visibleItems';

interface Props {
  connection: Connection;
  onAskAgent: () => void;
}

interface Row {
  rowId: string;
  issue: Issue;
  title: string;
  itemId: string;
  personaSlug: string;
  agentLabel: string;
  dismissedAt: string;
  reason: string;
  reasonLabel: string;
}

function rowFor(issue: Issue): Row {
  const dismissedAt = issue.dismissed_at ?? issue.updated_at;
  return {
    rowId: issue.id,
    issue,
    title: issue.title,
    itemId: issue.id.slice(0, 8).toUpperCase(),
    personaSlug: issue.persona ?? '',
    agentLabel: personaDisplayName(issue.persona),
    dismissedAt,
    reason: issue.dismiss_reason ?? '',
    reasonLabel: issue.dismiss_reason ? formatReason(issue.dismiss_reason) : '',
  };
}

function formatReason(reason: string): string {
  // Mirrors the Dismiss-dialog chip labels. Keep in sync with
  // DismissReason in api/client.ts.
  const map: Record<string, string> = {
    tone_off: 'Tone is off',
    wrong_product_focus: 'Wrong product focus',
    not_needed_now: 'Not needed now',
    write_myself: "I'll write this myself",
    wrong_timing: 'Wrong timing',
    out_of_stock: 'Out of stock',
    price_too_aggressive: 'Price too aggressive',
    needs_brand_review: 'Needs brand review',
    will_handle_myself: 'Will handle myself',
    other: 'Other',
  };
  return map[reason] ?? reason;
}

const DEFAULT_VIEW: View = {
  type: 'table',
  search: '',
  page: 1,
  perPage: 25,
  titleField: 'title',
  fields: ['agent', 'dismissed', 'reason'],
  layout: {
    density: 'comfortable',
  },
};

export default function Archived({ connection, onAskAgent }: Props) {
  const nav = useNavigate();
  const [issues, setIssues] = useState<Issue[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [view, setView] = useState<View>(DEFAULT_VIEW);

  useAskAgentContext(
    () => ({
      page: 'archived',
      visible_items: (issues ?? [])
        .filter((i) => i.status === 'dismissed' || i.status === 'rejected')
        .map(issueToVisible),
    }),
    [issues],
  );

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.issues(connection);
        if (cancelled) return;
        setIssues(res.issues);
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  const rows = useMemo<Row[]>(() => {
    if (!issues) return [];
    return issues
      .filter((i) => i.status === 'dismissed' || i.status === 'rejected')
      .sort((a, b) => {
        const aT = a.dismissed_at ?? a.updated_at;
        const bT = b.dismissed_at ?? b.updated_at;
        return bT.localeCompare(aT);
      })
      .map(rowFor);
  }, [issues]);

  const agentElements = useMemo(
    () => personaElementsFrom(rows.map((r) => r.personaSlug)),
    [rows],
  );

  const fields = useMemo<Field<Row>[]>(
    () => [
      {
        id: 'title',
        label: 'Proposal',
        enableHiding: false,
        enableGlobalSearch: true,
        getValue: ({ item }) => item.title,
        render: ({ item }) => (
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
              }}
            >
              {item.title}
            </Text>
            <span
              className="wa-mono"
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                color: 'var(--wpds-color-foreground-content-neutral-weak)',
              }}
            >
              {item.itemId}
            </span>
          </Stack>
        ),
      },
      {
        id: 'agent',
        label: 'Agent',
        getValue: ({ item }) => item.personaSlug,
        elements: agentElements,
        filterBy: { operators: ['isAny'] },
        render: ({ item }) => (
          <Stack direction="row" gap="xs" align="center">
            <PersonaAvatar persona={personaKeyFrom(item.personaSlug)} size="sm" />
            <Text variant="body-sm">{item.agentLabel}</Text>
          </Stack>
        ),
      },
      {
        id: 'dismissed',
        label: 'Dismissed',
        enableSorting: true,
        getValue: ({ item }) => item.dismissedAt,
        render: ({ item }) => (
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-foreground-content-neutral-weak)' }}
          >
            {relativeTime(item.dismissedAt)}
          </Text>
        ),
      },
      {
        id: 'reason',
        label: 'Reason',
        enableSorting: false,
        getValue: ({ item }) => item.reasonLabel,
        render: ({ item }) =>
          item.reasonLabel ? (
            <Badge intent="none">{item.reasonLabel}</Badge>
          ) : (
            <Text
              variant="body-sm"
              style={{
                color: 'var(--wpds-color-foreground-content-neutral-weak)',
                fontStyle: 'italic',
              }}
            >
              No reason given
            </Text>
          ),
      },
    ],
    [agentElements],
  );

  const actions = useMemo<Action<Row>[]>(() => [], []);

  const { data: shaped, paginationInfo } = useMemo(
    () => filterSortAndPaginate(rows, view, fields),
    [rows, view, fields],
  );

  if (error) {
    return (
      <Page
        title="Archived"
        subTitle="Proposals you dismissed. Kept for 30 days."
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <Notice.Root intent="error">
          <Notice.Description>Failed to load issues: {error}</Notice.Description>
        </Notice.Root>
      </Page>
    );
  }
  if (issues === null) {
    return (
      <Page
        title="Archived"
        subTitle="Proposals you dismissed. Kept for 30 days."
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Loading issues…</Text>
        </Stack>
      </Page>
    );
  }

  const empty = (
    <Stack
      direction="column"
      gap="sm"
      align="center"
      style={{ padding: 'var(--wpds-dimension-padding-3xl) 0' }}
    >
      <Text variant="heading-md">No dismissed proposals yet</Text>
      <Text
        variant="body-sm"
        style={{
          color: 'var(--wpds-color-foreground-content-neutral-weak)',
          textAlign: 'center',
          maxWidth: '420px',
        }}
      >
        When you dismiss a proposal, it lands here. Dismissed proposals are
        kept for 30 days before they're permanently deleted.
      </Text>
    </Stack>
  );

  return (
    <Page
      title="Archived"
      subTitle="Proposals you dismissed. Kept for 30 days."
      actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
      hasPadding
    >
      <DataViews<Row>
        view={view}
        onChangeView={setView}
        fields={fields}
        actions={actions}
        data={shaped}
        getItemId={(r) => r.rowId}
        paginationInfo={paginationInfo}
        defaultLayouts={{ table: {} }}
        onClickItem={(r) => nav(`/issues/${r.issue.id}`)}
        empty={empty}
      />
    </Page>
  );
}
