// Connection settings live in localStorage so reloads are instant and the UI
// can work against any daemon (local, teammate's machine, agency-managed) per
// the PRD §6.1 / §9. The daemon URL and token are never sent anywhere other
// than the daemon itself.
//
// When the UI is served by the daemon itself (release-binary install, single
// process), the daemon templates a per-run session token into index.html as
// `window.__WOOAGENT_TOKEN__`. That gets first priority — the user never sees
// Step 1's URL+token form. When loaded from a different origin (Vite dev on
// :5173, hosted UI later), the token isn't there and the manual form takes
// over via the localStorage fallback.

const STORAGE_KEY = 'wooagent.connection';

export interface Connection {
  daemonUrl: string;
  token: string;
}

declare global {
  interface Window {
    __WOOAGENT_TOKEN__?: string;
  }
}

// True when the daemon templated a per-run session token into index.html
// (it serves the UI itself at the daemon origin). In that mode, the
// connection is fully determined by the daemon process — localStorage
// can't override it and "forgetting" it via the UI is meaningless.
export function isEmbedded(): boolean {
  return typeof window !== 'undefined' && !!window.__WOOAGENT_TOKEN__;
}

export function loadConnection(): Connection | null {
  if (typeof window !== 'undefined' && window.__WOOAGENT_TOKEN__) {
    return {
      daemonUrl: window.location.origin,
      token: window.__WOOAGENT_TOKEN__,
    };
  }
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    const c = JSON.parse(raw) as Connection;
    if (!c.daemonUrl || !c.token) return null;
    return c;
  } catch {
    return null;
  }
}

export function saveConnection(c: Connection): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(c));
}

export function clearConnection(): void {
  localStorage.removeItem(STORAGE_KEY);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(
  connection: Connection,
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const url = connection.daemonUrl.replace(/\/$/, '') + path;
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(init.headers as Record<string, string> | undefined),
  };
  if (init.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  if (!path.startsWith('/v1/health')) {
    headers.Authorization = `Bearer ${connection.token}`;
  }
  const res = await fetch(url, { ...init, headers });
  if (!res.ok) {
    let code = 'http_error';
    let message = `${res.status} ${res.statusText}`;
    try {
      const payload = await res.json();
      if (payload?.error) {
        code = payload.error.code ?? code;
        message = payload.error.message ?? message;
      }
    } catch {
      /* body may not be JSON */
    }
    throw new ApiError(res.status, code, message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export interface Health {
  status: string;
  version: string;
  schema_version: string;
}

export interface Persona {
  persona: string;
  name: string;
  model_preference?: string;
  enabled: boolean;
  cadence_seconds?: number;
  max_attempts?: number;
  last_run_at?: string | null;
  next_run_at?: string | null;
}

// Run domain — scheduler activity feed. See daemon/internal/scheduler/*.
export type RunStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'skipped'
  | 'failed'
  | 'failed_permanent';

export type RunTrigger = 'tick' | 'manual' | 'bootstrap' | 'retry';
export type FailureClass = 'transient' | 'permanent' | '';

export interface Run {
  id: string;
  persona: string;
  trigger: RunTrigger;
  status: RunStatus;
  attempt: number;
  retry_of: string | null;
  scheduled_at: string;
  claimed_at: string | null;
  completed_at: string | null;
  latency_ms: number | null;
  issue_id: string | null;
  turn_id: string | null;
  skip_reason: string | null;
  failure_reason: string | null;
  failure_class: FailureClass;
}

export interface RunDetailResponse {
  run: Run;
  turn_event: unknown | null;
  retry_chain: Run[];
}

/** Canonical dismiss reasons exposed in the Dismiss dialog. Free-text
 *  `comment` carries any nuance the operator wants to add. */
export type DismissReason =
  | 'tone_off'
  | 'wrong_product_focus'
  | 'not_needed_now'
  | 'write_myself'
  | 'wrong_timing'
  | 'out_of_stock'
  | 'price_too_aggressive'
  | 'needs_brand_review'
  | 'will_handle_myself'
  | 'other';

export interface Issue {
  id: string;
  title: string;
  description?: string;
  persona?: string;
  status:
    | 'backlog'
    | 'todo'
    | 'in_progress'
    | 'in_review'
    | 'done'
    | 'rejected'
    | 'dismissed';
  /** Captured at dismiss time, surfaced on the Archive screen for prompt-tuning. */
  dismiss_reason?: DismissReason;
  /** Optional free-text the operator added in the Dismiss dialog. */
  dismiss_comment?: string;
  /** ISO-8601 — when the operator dismissed. Drives the "Dismissed" column +
   *  the 30-day TTL countdown on the Archive screen. */
  dismissed_at?: string;
  priority: 'urgent' | 'high' | 'medium' | 'low' | 'none';
  /** Set when this issue is part of a batch. Cards in the kanban that
   *  carry a batch_id route to /batches/:id instead of /issues/:id. */
  batch_id?: string;
  created_at: string;
  updated_at: string;
}

export interface Batch {
  id: string;
  title: string;
  persona?: string;
  intent?: string;
  source_run_id?: string;
  /** Counts derived at read-time on the daemon. Always reflects the
   *  children's current statuses — never stored, never out of sync. */
  total: number;
  pending: number;
  approved: number;
  rejected: number;
  created_at: string;
  updated_at: string;
}

export interface BatchDetail {
  batch: Batch;
  issues: { issue: Issue; proposal: Proposal | null }[];
}

export interface BatchApproveChild {
  issue_id: string;
  variant_id?: string;
}

export interface BatchOperationResult {
  /** Per-child outcome from a batch approve-all / reject-all. The daemon
   *  always returns 200 even if some children failed PEP — the per-child
   *  ok flag drives the UI. */
  results: Array<{
    issue_id: string;
    ok: boolean;
    status?: string;
    ability?: string;
    audit_id?: number;
    updated_at?: string;
    error?: { code: string; message: string };
  }>;
}

export interface Variant {
  id: string;
  label: string;
  body: string;
  seo: number;
  voice: number;
  charCount: number;
  recommended?: boolean;
  note?: string;
}

export interface Proposal {
  type: string;
  content: string;
  target?: Record<string, unknown>;
}

// Pull a typed variants list out of proposal.target.variants. Returns null
// when the proposal is single-shot (no variants array). Filters out
// malformed entries so the UI never has to defensively check shape.
export function variantsFromProposal(p: Proposal | null | undefined): Variant[] | null {
  if (!p?.target) return null;
  const raw = (p.target as Record<string, unknown>).variants;
  if (!Array.isArray(raw)) return null;
  const out: Variant[] = [];
  for (const v of raw) {
    if (!v || typeof v !== 'object') continue;
    const r = v as Record<string, unknown>;
    if (typeof r.id !== 'string' || typeof r.body !== 'string') continue;
    out.push({
      id: r.id,
      label: typeof r.label === 'string' ? r.label : r.id,
      body: r.body,
      seo: typeof r.seo === 'number' ? r.seo : 0,
      voice: typeof r.voice === 'number' ? r.voice : 0,
      charCount:
        typeof r.charCount === 'number' ? r.charCount : r.body.length,
      recommended: r.recommended === true,
      note: typeof r.note === 'string' ? r.note : undefined,
    });
  }
  return out.length > 0 ? out : null;
}

// Pricing-persona structured proposal shape. The pricing-benchmark skill
// emits these fields into proposal.target; the daemon's
// product_price_change approve dispatch reads target.regular_price to drive
// the wooagent-products/update call.
export interface PriceSource {
  url: string;
  retailer?: string;
  comparable_product: string;
  observed_price: number;
  currency?: string;
  note?: string;
}

export interface PriceProposal {
  productId?: number;
  productName?: string;
  productSku?: string;
  currency: string;
  previousPrice: number;
  proposedPrice: number;
  /** Signed. Negative = price cut. Daemon caps at ±25% per step. */
  percentChange: number;
  direction: 'increase' | 'decrease' | 'hold';
  observedLow?: number;
  observedMedian?: number;
  observedHigh?: number;
  sources: PriceSource[];
}

export function priceProposalFromProposal(
  p: Proposal | null | undefined,
): PriceProposal | null {
  if (!p || p.type !== 'product_price_change' || !p.target) return null;
  const t = p.target as Record<string, unknown>;
  const previous = typeof t.previous_price === 'number' ? t.previous_price : NaN;
  const proposed = typeof t.proposed_price === 'number' ? t.proposed_price : NaN;
  if (!Number.isFinite(previous) || !Number.isFinite(proposed)) return null;
  const direction =
    t.direction === 'increase' || t.direction === 'decrease' || t.direction === 'hold'
      ? (t.direction as PriceProposal['direction'])
      : proposed > previous
        ? 'increase'
        : proposed < previous
          ? 'decrease'
          : 'hold';
  const sourcesRaw = Array.isArray(t.sources) ? t.sources : [];
  const sources: PriceSource[] = [];
  for (const s of sourcesRaw) {
    if (!s || typeof s !== 'object') continue;
    const r = s as Record<string, unknown>;
    if (typeof r.url !== 'string' || typeof r.observed_price !== 'number') continue;
    sources.push({
      url: r.url,
      retailer: typeof r.retailer === 'string' ? r.retailer : undefined,
      comparable_product:
        typeof r.comparable_product === 'string' ? r.comparable_product : r.url,
      observed_price: r.observed_price,
      currency: typeof r.currency === 'string' ? r.currency : undefined,
      note: typeof r.note === 'string' ? r.note : undefined,
    });
  }
  return {
    productId: typeof t.product_id === 'number' ? t.product_id : undefined,
    productName: typeof t.product_name === 'string' ? t.product_name : undefined,
    productSku: typeof t.product_sku === 'string' ? t.product_sku : undefined,
    currency: typeof t.currency === 'string' ? t.currency : 'USD',
    previousPrice: previous,
    proposedPrice: proposed,
    percentChange:
      typeof t.percent_change === 'number'
        ? t.percent_change
        : ((proposed - previous) / previous) * 100,
    direction,
    observedLow: typeof t.observed_low === 'number' ? t.observed_low : undefined,
    observedMedian:
      typeof t.observed_median === 'number' ? t.observed_median : undefined,
    observedHigh: typeof t.observed_high === 'number' ? t.observed_high : undefined,
    sources,
  };
}

// Sales Support structured proposal shape. The persona emits these fields
// into proposal.target; the daemon's customer_reply_draft approve dispatch
// reads them to call wooagent-orders/add-note.
export interface MessageLineItem {
  product_id?: number;
  name: string;
  quantity?: number;
  total?: string;
  sku?: string;
}

export interface MessageProposal {
  message: string; // body of the note (plain text, may include \n)
  noteType: 'customer' | 'internal';
  subjectHint?: string;
  orderId: number;
  orderNumber?: string;
  orderStatus?: string;
  orderTotal?: string;
  orderCurrency?: string;
  orderDate?: string;
  customerId?: number;
  customerEmail?: string;
  customerName?: string;
  lineItems: MessageLineItem[];
}

export function messageProposalFromProposal(
  p: Proposal | null | undefined,
): MessageProposal | null {
  if (!p || p.type !== 'customer_reply_draft' || !p.target) return null;
  const t = p.target as Record<string, unknown>;
  const orderId = typeof t.order_id === 'number' ? t.order_id : NaN;
  if (!Number.isFinite(orderId)) return null;
  const noteTypeRaw =
    typeof t.note_type === 'string' ? t.note_type.toLowerCase() : 'customer';
  const noteType: MessageProposal['noteType'] =
    noteTypeRaw === 'internal' ? 'internal' : 'customer';

  const itemsRaw = Array.isArray(t.line_items) ? t.line_items : [];
  const lineItems: MessageLineItem[] = [];
  for (const it of itemsRaw) {
    if (!it || typeof it !== 'object') continue;
    const r = it as Record<string, unknown>;
    if (typeof r.name !== 'string') continue;
    lineItems.push({
      product_id: typeof r.product_id === 'number' ? r.product_id : undefined,
      name: r.name,
      quantity: typeof r.quantity === 'number' ? r.quantity : undefined,
      total: typeof r.total === 'string' ? r.total : undefined,
      sku: typeof r.sku === 'string' ? r.sku : undefined,
    });
  }

  return {
    message: p.content,
    noteType,
    subjectHint: typeof t.subject_hint === 'string' ? t.subject_hint : undefined,
    orderId,
    orderNumber:
      typeof t.order_number === 'string' ? t.order_number : undefined,
    orderStatus:
      typeof t.order_status === 'string' ? t.order_status : undefined,
    orderTotal: typeof t.order_total === 'string' ? t.order_total : undefined,
    orderCurrency:
      typeof t.order_currency === 'string' ? t.order_currency : undefined,
    orderDate: typeof t.order_date === 'string' ? t.order_date : undefined,
    customerId:
      typeof t.customer_id === 'number' ? t.customer_id : undefined,
    customerEmail:
      typeof t.customer_email === 'string' ? t.customer_email : undefined,
    customerName:
      typeof t.customer_name === 'string' ? t.customer_name : undefined,
    lineItems,
  };
}

export interface IssueDetail {
  issue: Issue;
  runs: unknown[];
  proposal: Proposal | null;
}

// Onboarding domain types — see onboarding-design-brief.md §10. The daemon
// surface for these is sequenced after the brief (engineering plan #2);
// until it lands, these endpoints will 404 and the UI surfaces ApiError.

export interface Store {
  id: string;
  url: string;
  mcp_endpoint?: string;
  device_name?: string;
  status: 'pairing' | 'paired' | 'expired' | 'failed';
  pairing_code?: string;
  pair_url?: string;
  expires_at?: string;
  paired_at?: string;
  ability_count?: number;
  last_discovered_at?: string;
}

export type ModelProviderKind = 'anthropic' | 'openai' | 'ollama';

export interface ModelProvider {
  id: string;
  kind: ModelProviderKind;
  default_model: string;
  endpoint?: string;
  // Display name. Defaults to "${title} · ${default_model}" but operators
  // can rename — meaningful when multiple configurations of the same
  // kind exist (e.g. "Anthropic · prod" vs "Anthropic · staging").
  name?: string;
  // The fleet default. Personas without an explicit override fall back
  // to this provider. Exactly one row is marked default.
  is_default?: boolean;
  last_tested_at?: string;
  last_test_status?: 'ok' | 'failed' | 'untested';
}

export interface ModelTestRequest {
  kind: ModelProviderKind;
  api_key?: string;
  endpoint?: string;
  default_model?: string;
}

export interface ModelTestResult {
  ok: boolean;
  message?: string;
  /** Populated for kinds that auto-discover models (Ollama, OpenAI list). */
  models?: string[];
}

export interface ModelProviderCreate {
  kind: ModelProviderKind;
  api_key?: string;
  endpoint?: string;
  default_model: string;
}

export interface ApproveResult {
  id: string;
  status: 'done' | 'rejected';
  ability?: string;
  updated_at: string;
}

// Abilities domain — discovered per paired store via the WP MCP Adapter.
// trust_state is the daemon's three-state machine; the UI surfaces it as
// a Badge intent and uses it to gate the per-row "Trust" action.
export type AbilityTrustState = 'new' | 'trusted' | 'schema_changed';

export type AbilityEffectiveTrust =
  | 'built-in'
  | 'trusted'
  | 'needs_review'
  | 'schema_changed'
  | 'revoked';

export interface Ability {
  id: string;
  store_id: string;
  store_url?: string;
  name: string;
  title?: string;
  description?: string;
  version?: string;
  /** Cached MCP get-ability-info envelope. JSON-shaped; surfaced raw in
   *  the inspector for V1 (no structured schema renderer yet). */
  schema?: Record<string, unknown>;
  schema_hash?: string;
  trust_state: AbilityTrustState;
  effective_trust?: AbilityEffectiveTrust;
  revoked_at?: string;
  revoked_by?: string;
  trusted_at?: string;
  last_seen_at?: string;
}

export const api = {
  health: (c: Connection) => request<Health>(c, '/v1/health'),
  agents: (c: Connection) => request<{ agents: Persona[] }>(c, '/v1/agents'),
  runs: {
    list: (
      c: Connection,
      params: {
        persona?: string;
        status?: string[];
        issueId?: string;
        limit?: number;
        cursor?: string;
      } = {},
    ) => {
      const q = new URLSearchParams();
      if (params.persona) q.set('persona', params.persona);
      if (params.status?.length) q.set('status', params.status.join(','));
      if (params.issueId) q.set('issue_id', params.issueId);
      if (params.limit) q.set('limit', String(params.limit));
      if (params.cursor) q.set('cursor', params.cursor);
      const suffix = q.toString() ? `?${q.toString()}` : '';
      return request<{ runs: Run[]; next_cursor: string | null }>(
        c,
        `/v1/runs${suffix}`,
      );
    },
    get: (c: Connection, id: string) =>
      request<RunDetailResponse>(c, `/v1/runs/${id}`),
    trigger: (c: Connection, persona: string) =>
      request<{ run: Run }>(c, '/v1/runs', {
        method: 'POST',
        body: JSON.stringify({ persona }),
      }),
  },
  issues: (c: Connection) => request<{ issues: Issue[] }>(c, '/v1/issues'),
  issue: (c: Connection, id: string) => request<IssueDetail>(c, `/v1/issues/${id}`),
  createIssue: (c: Connection, body: Partial<Issue>) =>
    request<Issue>(c, '/v1/issues', { method: 'POST', body: JSON.stringify(body) }),
  approve: (c: Connection, id: string, variantId?: string) =>
    request<ApproveResult>(c, `/v1/issues/${id}/approve`, {
      method: 'POST',
      body: variantId ? JSON.stringify({ variant_id: variantId }) : undefined,
    }),
  reject: (c: Connection, id: string) =>
    request<ApproveResult>(c, `/v1/issues/${id}/reject`, { method: 'POST' }),
  dismiss: (
    c: Connection,
    id: string,
    body: { reason: DismissReason; comment?: string },
  ) =>
    request<ApproveResult>(c, `/v1/issues/${id}/dismiss`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  batches: {
    list: (c: Connection) => request<{ batches: Batch[] }>(c, '/v1/batches'),
    get: (c: Connection, id: string) => request<BatchDetail>(c, `/v1/batches/${id}`),
    approveAll: (c: Connection, id: string, children: BatchApproveChild[]) =>
      request<BatchOperationResult>(c, `/v1/batches/${id}/approve-all`, {
        method: 'POST',
        body: JSON.stringify({ children }),
      }),
    rejectAll: (c: Connection, id: string) =>
      request<BatchOperationResult>(c, `/v1/batches/${id}/reject-all`, {
        method: 'POST',
      }),
  },
  stores: {
    list: (c: Connection) => request<{ stores: Store[] }>(c, '/v1/stores'),
    get: (c: Connection, id: string) => request<Store>(c, `/v1/stores/${id}`),
    create: (c: Connection, url: string) =>
      request<Store>(c, '/v1/stores', {
        method: 'POST',
        body: JSON.stringify({ url }),
      }),
    delete: (c: Connection, id: string) =>
      request<void>(c, `/v1/stores/${id}`, { method: 'DELETE' }),
  },
  abilities: {
    list: (
      c: Connection,
      params?: { storeId?: string; trustState?: AbilityTrustState },
    ) => {
      const qs = new URLSearchParams();
      if (params?.storeId) qs.set('store_id', params.storeId);
      if (params?.trustState) qs.set('trust_state', params.trustState);
      const suffix = qs.toString() ? `?${qs.toString()}` : '';
      return request<{ abilities: Ability[] }>(c, `/v1/abilities${suffix}`);
    },
    trust: (c: Connection, id: string) =>
      request<Ability>(c, `/v1/abilities/${id}/trust`, { method: 'POST' }),
    revoke: (c: Connection, id: string) =>
      request<Ability>(c, `/v1/abilities/${id}/revoke`, { method: 'POST' }),
    restore: (c: Connection, id: string) =>
      request<Ability>(c, `/v1/abilities/${id}/restore`, { method: 'POST' }),
  },
  modelProviders: {
    list: (c: Connection) =>
      request<{ providers: ModelProvider[] }>(c, '/v1/model-providers'),
    // Pre-save validation. Engineering plan #2 sketches POST
    // /v1/model-providers/:id/test (post-save) — but Step 4's "Test
    // connection must pass before Save" requirement needs a no-id endpoint
    // so we don't litter the daemon with untested providers. This shape is
    // the UI's ask of engineering.
    test: (c: Connection, body: ModelTestRequest) =>
      request<ModelTestResult>(c, '/v1/model-providers/test', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    create: (c: Connection, body: ModelProviderCreate) =>
      request<ModelProvider>(c, '/v1/model-providers', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    delete: (c: Connection, id: string) =>
      request<void>(c, `/v1/model-providers/${id}`, { method: 'DELETE' }),
  },
};
