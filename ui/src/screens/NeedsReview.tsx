import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Badge, Notice, Stack, Text } from '@wordpress/ui';
import { Snackbar, Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import { type Batch, type Issue } from '../api/client';
import ProductThumbnail from '../components/ProductThumbnail';
import { kindFromIssue, kindFromPersonaSlug } from '../components/StatusBadge';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
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

type ToastState = { kind: 'success' | 'error'; text: string };

// Flat shape that DataViews consumes. Discriminator + denormalized fields
// (title, persona, kind, etc.) so each column has a plain value to render
// without poking through the BoardItem union at every field.
interface Row {
  rowId: string;
  origin: BoardItem;
  // Denormalized for filtering / sorting / search.
  title: string;
  personaSlug: string;
  agentLabel: string;
  /** Item identifier shown in the eyebrow. WOO-IDs for issues, "BATCH · …"
   *  for batches. Kept as a separate column so the table layout can show it. */
  itemId: string;
  /** Count of children for batch rows (drives the "N products" badge);
   *  undefined for single-issue rows. */
  batchCount?: number;
  updatedAt: string;
  /** Meta line shown under the title in grid view ("3 variants",
   *  "5 of 11 price changes pending", etc.). */
  meta: string;
  imageUrl?: string;
  imageAlt?: string;
}

function metaForBoardItem(item: BoardItem): string {
  if (item.kind === 'batch') {
    const b = item.batch;
    const kind = kindFromPersonaSlug(b.persona);
    const noun = kindNoun(kind, b.total);
    if (b.pending > 0) {
      return `${b.pending} of ${b.total} ${noun} pending`;
    }
    if (b.approved === b.total) {
      return `${b.total} ${noun} approved`;
    }
    if (b.approved > 0) {
      return `${b.approved}/${b.total} ${noun} approved`;
    }
    return `${b.total} ${noun}`;
  }
  const issue = item.issue;
  const kind = kindFromIssue(issue);
  if (kind === 'content') return '3 variants';
  if (kind === 'price') return 'price change';
  if (kind === 'message') return 'reply draft';
  return '1 item';
}

function kindNoun(kind: ReturnType<typeof kindFromIssue>, count: number): string {
  switch (kind) {
    case 'content':
      return count === 1 ? 'rewrite' : 'rewrites';
    case 'campaign':
      return count === 1 ? 'campaign' : 'campaigns';
    case 'email':
      return count === 1 ? 'email' : 'emails';
    case 'price':
      return count === 1 ? 'price change' : 'price changes';
    case 'message':
      return count === 1 ? 'reply' : 'replies';
  }
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
      updatedAt: b.updated_at,
      meta: metaForBoardItem(item),
      // Batch.target is not surfaced by /v1/batches yet — B2 known limitation.
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
    updatedAt: issue.updated_at,
    meta: metaForBoardItem(item),
    imageUrl: typeof issue.target?.image_url === 'string' ? issue.target.image_url : undefined,
    imageAlt: typeof issue.target?.image_alt === 'string' ? issue.target.image_alt : undefined,
  };
}

// DataViews default view. Grid is the canonical layout for the queue —
// operators scan agent proposals visually by persona color. Table is
// available via the layout toggle for high-volume sessions.
function buildDefaultView(initialPersona: string | null): View {
  return {
    type: 'grid',
    search: '',
    page: 1,
    perPage: 24,
    titleField: 'title',
    descriptionField: 'meta',
    mediaField: 'image',
    fields: ['agent', 'age', 'image'],
    filters: initialPersona
      ? [{ field: 'agent', operator: 'isAny', value: [initialPersona] }]
      : [],
    layout: {
      badgeFields: ['itemId'],
      density: 'comfortable',
    },
  };
}

export default function NeedsReview({
  issues,
  batches,
  error,
  onAskAgent,
}: Props) {
  const nav = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const initialPersona = searchParams.get('persona');
  const [toast, setToast] = useState<ToastState | null>(null);
  // `?persona=X` deep-links from Agents → "View issues" seed the agent
  // filter on first mount. Subsequent user changes to the filter live
  // in DataViews state and don't write back to the URL.
  const [view, setView] = useState<View>(() => buildDefaultView(initialPersona));

  // Toast handoff from IssueDetail / BatchReview (same pattern as the old
  // Kanban): read once, then clear history state so refresh doesn't repeat.
  useEffect(() => {
    const incoming = (location.state as { toast?: ToastState } | null)?.toast;
    if (incoming) {
      setToast(incoming);
      nav(location.pathname + location.search, {
        replace: true,
        state: null,
      });
    }
  }, [location, nav]);

  const title = "Today's queue";
  const subTitle =
    "Everything your agents have staged for you. Approve, adjust, or let them work.";

  const rows = useMemo<Row[]>(() => {
    if (!issues) return [];
    return buildBoardItems(issues, batches, 'in_review').map(rowFor);
  }, [issues, batches]);

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
        ),
      },
      {
        id: 'itemId',
        label: 'ID',
        enableSorting: false,
        getValue: ({ item }) => item.itemId,
        render: ({ item }) => (
          <span
            className="wa-mono"
            style={{
              fontSize: 'var(--wpds-typography-font-size-xs)',
              color: 'var(--wpds-color-fg-content-neutral-weak)',
            }}
          >
            {item.itemId}
          </span>
        ),
      },
      {
        id: 'meta',
        label: 'Details',
        enableSorting: false,
        enableHiding: false,
        getValue: ({ item }) => item.meta,
        render: ({ item }) => (
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            {item.meta}
          </Text>
        ),
      },
      {
        id: 'agent',
        label: 'Agent',
        // Slug, not display name — the filter compares the field value
        // against `elements.value` (slug) on each row.
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
        id: 'age',
        label: 'Age',
        enableSorting: true,
        getValue: ({ item }) => item.updatedAt,
        render: ({ item }) => (
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            {relativeTime(item.updatedAt)}
          </Text>
        ),
      },
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
    ],
    [agentElements],
  );

  const actions = useMemo<Action<Row>[]>(() => [], []);

  const { data: shaped, paginationInfo } = useMemo(
    () => filterSortAndPaginate(rows, view, fields),
    [rows, view, fields],
  );

  const snackbar = toast ? (
    <div className="wa-snackbar-host">
      <Snackbar onRemove={() => setToast(null)}>{toast.text}</Snackbar>
    </div>
  ) : null;

  if (error) {
    return (
      <>
        <Page
          title={title}
          subTitle={subTitle}
          actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
          hasPadding
        >
          <Notice.Root intent="error">
            <Notice.Description>
              Failed to load issues: {error}
            </Notice.Description>
          </Notice.Root>
        </Page>
        {snackbar}
      </>
    );
  }
  if (issues === null) {
    return (
      <>
        <Page
          title={title}
          subTitle={subTitle}
          actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
          hasPadding
        >
          <Stack direction="row" gap="sm" align="center">
            <Spinner /> <Text variant="body-sm">Loading issues…</Text>
          </Stack>
        </Page>
        {snackbar}
      </>
    );
  }

  const empty = (
    <Stack
      direction="column"
      gap="sm"
      align="center"
      style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}
    >
      <Text variant="heading-md">Inbox zero.</Text>
      <Text
        variant="body-sm"
        style={{
          color: 'var(--wpds-color-fg-content-neutral-weak)',
          textAlign: 'center',
          maxWidth: '420px',
        }}
      >
        Your agents are quiet right now. New proposals will land here when they finish their next run.
      </Text>
    </Stack>
  );

  return (
    <>
      <Page
        title={title}
        subTitle={subTitle}
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
          defaultLayouts={{ grid: {}, table: {} }}
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
      {snackbar}
    </>
  );
}
