import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Badge, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews';
import type { Action, Field, View } from '@wordpress/dataviews';
import { type Batch, type Issue } from '../api/client';
import ProductThumbnail from '../components/ProductThumbnail';
import ActionSnackbar from '../components/ActionSnackbar';
import { kindFromIssue, kindFromPersonaSlug, type IssueKind } from '../components/StatusBadge';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import PageGlobalActions from '../components/PageGlobalActions';
import {
  buildBoardItems,
  personaDisplayName,
  personaElementsFrom,
  relativeTime,
  type BoardItem,
} from '../lib/boardItems';
import { useAskAgentContext } from '../lib/askAgent';
import { batchToVisible, issueToVisible } from '../lib/visibleItems';

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
  /** Human-readable proposal type, shown as a labeled row on the queue
   *  card ("Price change", "Reply draft", "Bulk price changes", etc.).
   *  Derived from persona + a small set of title heuristics for sub-types.
   *  See proposalLabel(). */
  proposal: string;
  imageUrl?: string;
  imageAlt?: string;
}

// Coarse single-proposal label per persona. The DataViews "Proposal"
// column reads this. Sub-typing for Marketing ("Title rewrite" vs the
// default "Product description rewrite"), Pricing, etc. is a Phase B
// follow-up that needs the daemon to surface proposal.type on /v1/issues
// — for now we keep one label per persona since Phase 1 emits only
// uniform proposal types per agent.
function proposalLabelForKind(kind: IssueKind): string {
  switch (kind) {
    case 'price':
      return 'Price change';
    case 'message':
      return 'Reply draft';
    case 'inventory':
      return 'Low-stock alert';
    case 'accounting':
      return 'Bookkeeping check';
    case 'report':
      return 'Sales digest';
    case 'summary':
      return 'Daily summary';
    case 'campaign':
      return 'Marketing campaign';
    case 'email':
      return 'Email draft';
    case 'content':
    default:
      return 'Product description rewrite';
  }
}

// Batch variant — same set with "Bulk" prefix or a more natural plural
// where it reads better. Surfaces in the Proposal field for batch rows.
function batchProposalLabelForKind(kind: IssueKind): string {
  switch (kind) {
    case 'price':
      return 'Bulk price changes';
    case 'message':
      return 'Bulk replies';
    case 'inventory':
      return 'Bulk restock';
    case 'accounting':
      return 'Bulk reconciliation';
    case 'campaign':
      return 'Marketing campaigns';
    case 'email':
      return 'Email batch';
    case 'report':
    case 'summary':
    case 'content':
    default:
      return 'Bulk content rewrites';
  }
}

function proposalLabel(item: BoardItem): string {
  if (item.kind === 'batch') {
    return batchProposalLabelForKind(kindFromPersonaSlug(item.batch.persona));
  }
  return proposalLabelForKind(kindFromIssue(item.issue));
}

// Reporting and Chief proposals don't have a product/order "object" the
// way Marketing/Pricing/Sales-support do — their cards make more sense
// titled by the period they cover. Phase B (daemon-side target.object_label)
// will give us a proper object for the rest; Reporting/Chief will
// continue to derive their title client-side.
function titleForRow(item: BoardItem): string {
  if (item.kind === 'batch') return item.batch.title;
  const issue = item.issue;
  const kind = kindFromIssue(issue);
  if (kind === 'report') return weekTitle(issue.created_at ?? issue.updated_at);
  if (kind === 'summary') return dayTitle(issue.created_at ?? issue.updated_at);
  return issue.title;
}

function weekTitle(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return 'Weekly digest';
  // Snap to the Monday of the issue's week so cards within the same
  // weekly cycle group under the same title.
  const day = d.getDay();
  const monday = new Date(d);
  monday.setDate(d.getDate() - ((day + 6) % 7));
  return `Week of ${monday.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
}

function dayTitle(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return 'Daily summary';
  const today = new Date();
  const sameDay = (a: Date, b: Date) =>
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate();
  if (sameDay(d, today)) return 'Today';
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (sameDay(d, yesterday)) return 'Yesterday';
  return d.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });
}

function rowFor(item: BoardItem): Row {
  if (item.kind === 'batch') {
    const b = item.batch;
    return {
      rowId: `batch:${b.id}`,
      origin: item,
      title: titleForRow(item),
      personaSlug: b.persona ?? '',
      agentLabel: personaDisplayName(b.persona),
      itemId: `BATCH · ${b.id.slice(0, 6).toUpperCase()}`,
      batchCount: b.total,
      updatedAt: b.updated_at,
      proposal: proposalLabel(item),
      // Batch.target is not surfaced by /v1/batches yet — B2 known limitation.
      imageUrl: undefined,
      imageAlt: undefined,
    };
  }
  const issue = item.issue;
  return {
    rowId: `issue:${issue.id}`,
    origin: item,
    // TODO(Phase B): once the daemon surfaces target.object_label, prefer
    // it over issue.title here so the card title shows just the object
    // (e.g., "Cap") instead of the combined daemon string
    // ("Cap $16.00 → $20.00 (+16.7%)"). Reporting/Chief already get a
    // date-derived title via titleForRow.
    title: titleForRow(item),
    personaSlug: issue.persona ?? '',
    agentLabel: personaDisplayName(issue.persona),
    itemId: issue.id.slice(0, 8).toUpperCase(),
    updatedAt: issue.updated_at,
    proposal: proposalLabel(item),
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
    // No description field — the proposal type now lives in a labeled
    // "Proposal" row (see fields below), per the i3 Figma card layout.
    mediaField: 'image',
    fields: ['proposal', 'agent', 'time'],
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

  // Publish current visible items + page tag for the Ask Agent drawer
  // (DSGWOO-1348 B4). Includes both standalone in-review issues and
  // batches (which collapse multiple issues into one row).
  useAskAgentContext(
    () => ({
      page: 'needs-review',
      visible_items: [
        ...(issues ?? [])
          .filter((i) => i.status === 'in_review')
          .map(issueToVisible),
        ...batches.filter((b) => b.pending > 0).map(batchToVisible),
      ],
    }),
    [issues, batches],
  );

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
            {/* min-width: 0 + flex: 1 lets the title shrink below its
                intrinsic width so the ellipsis triplet can kick in. The
                250px-wide grid tile would otherwise stretch the whole
                Stack horizontally and leave the title untruncated. */}
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
                flex: '1 1 auto',
                minWidth: 0,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
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
        // id stays 'proposal' to match Row.proposal / proposalLabel(); only
        // the display label changes. The title column above is already
        // labeled "Proposal" in the table view (DataViews uses the
        // titleField's column header), so this one becomes "Type" to
        // disambiguate.
        id: 'proposal',
        label: 'Type',
        enableSorting: false,
        enableHiding: false,
        getValue: ({ item }) => item.proposal,
        render: ({ item }) => (
          <Text variant="body-sm">{item.proposal}</Text>
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
        id: 'time',
        label: 'Time',
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
            size="sm"
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
    <ActionSnackbar text={toast.text} onRemove={() => setToast(null)} />
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
