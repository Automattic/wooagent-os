import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import { type Batch, type Issue } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import ProductThumbnail from '../components/ProductThumbnail';
import PageGlobalActions from '../components/PageGlobalActions';
import {
  buildBoardItems,
  personaDisplayName,
  personaElementsFrom,
  relativeTime,
  type BoardItem,
} from '../lib/boardItems';

interface Props {
  issues: Issue[] | null;
  batches: Batch[];
  error: string | null;
  onAskAgent: () => void;
}

interface Row {
  rowId: string;
  origin: BoardItem;
  title: string;
  personaSlug: string;
  agentLabel: string;
  itemId: string;
  batchCount?: number;
  /** Most-recent activity timestamp. For Done items, this is when the
   *  operator approved (or when the daemon flipped the issue to done). */
  completedAt: string;
  imageUrl?: string;
  imageAlt?: string;
}

function rowFor(item: BoardItem): Row {
  if (item.kind === 'batch') {
    const b = item.batch;
    return {
      rowId: `batch:${b.id}`,
      origin: item,
      title: b.title,
      personaSlug: b.persona ?? '',
      agentLabel: personaDisplayName(b.persona),
      itemId: `BATCH · ${b.id.slice(0, 6).toUpperCase()}`,
      batchCount: b.total,
      completedAt: b.updated_at,
      // Batch rows don't carry target yet — Batch type lacks `target` on the
      // wire. Cards show the persona-derived placeholder. Revisit when
      // /v1/batches surfaces target.
      imageUrl: undefined,
      imageAlt: undefined,
    };
  }
  const issue = item.issue;
  return {
    rowId: `issue:${issue.id}`,
    origin: item,
    title: issue.title,
    personaSlug: issue.persona ?? '',
    agentLabel: personaDisplayName(issue.persona),
    itemId: issue.id.slice(0, 8).toUpperCase(),
    completedAt: issue.updated_at,
    imageUrl: typeof issue.target?.image_url === 'string' ? issue.target.image_url : undefined,
    imageAlt: typeof issue.target?.image_alt === 'string' ? issue.target.image_alt : undefined,
  };
}

const DEFAULT_VIEW: View = {
  type: 'table',
  search: '',
  page: 1,
  perPage: 25,
  titleField: 'title',
  mediaField: 'image',
  fields: ['agent', 'completed'],
  layout: {
    density: 'comfortable',
  },
};

export default function Done({ issues, batches, error, onAskAgent }: Props) {
  const nav = useNavigate();
  const [view, setView] = useState<View>(DEFAULT_VIEW);

  const rows = useMemo<Row[]>(() => {
    if (!issues) return [];
    return buildBoardItems(issues, batches, 'done').map(rowFor);
  }, [issues, batches]);

  const agentElements = useMemo(
    () => personaElementsFrom(rows.map((r) => r.personaSlug)),
    [rows],
  );

  const fields = useMemo<Field<Row>[]>(
    () => [
      {
        id: 'image',
        label: 'Image',
        enableHiding: false,
        enableSorting: false,
        getValue: ({ item }) => item.imageUrl ?? '',
        render: ({ item }) => (
          <ProductThumbnail
            src={item.imageUrl}
            alt={item.imageAlt}
            persona={item.personaSlug}
            size="md"
          />
        ),
      },
      {
        id: 'title',
        label: 'Proposal',
        enableHiding: false,
        enableGlobalSearch: true,
        getValue: ({ item }) => item.title,
        render: ({ item }) => (
          <Stack direction="column" gap="xs">
            <Stack direction="row" gap="xs" align="center">
              <Text
                variant="body-sm"
                style={{
                  fontWeight: 'var(--wpds-typography-font-weight-medium)',
                }}
              >
                {item.title}
              </Text>
              {item.batchCount !== undefined && (
                <Badge intent="informational">
                  {`${item.batchCount} products`}
                </Badge>
              )}
            </Stack>
            <span
              className="wa-mono"
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                color: 'var(--wpds-color-fg-content-neutral-weak)',
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
        id: 'completed',
        label: 'Completed',
        enableSorting: true,
        getValue: ({ item }) => item.completedAt,
        render: ({ item }) => (
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            {relativeTime(item.completedAt)}
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
        title="Done"
        subTitle="Proposals you've approved or that have shipped."
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
        title="Done"
        subTitle="Proposals you've approved or that have shipped."
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
      style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}
    >
      <Text variant="heading-md">Nothing shipped yet.</Text>
      <Text
        variant="body-sm"
        style={{
          color: 'var(--wpds-color-fg-content-neutral-weak)',
          textAlign: 'center',
          maxWidth: '420px',
        }}
      >
        Approved proposals will land here. Approvals stay reversible for a window
        — open one to undo.
      </Text>
    </Stack>
  );

  return (
    <Page
      title="Done"
      subTitle="Proposals you've approved or that have shipped."
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
        defaultLayouts={{ table: {}, grid: {} }}
        onClickItem={(r) => {
          if (r.origin.kind === 'batch') {
            nav(`/batches/${r.origin.batch.id}`);
            return;
          }
          const issue = r.origin.issue;
          if (issue.batch_id) {
            nav(`/batches/${issue.batch_id}`);
          } else {
            nav(`/issues/${issue.id}`);
          }
        }}
        empty={empty}
      />
    </Page>
  );
}
