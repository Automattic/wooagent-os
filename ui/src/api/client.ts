// Connection settings live in localStorage so reloads are instant and the UI
// can work against any daemon (local, teammate's machine, agency-managed) per
// the PRD §6.1 / §9. The daemon URL and token are never sent anywhere other
// than the daemon itself.

const STORAGE_KEY = 'wooagent.connection';

export interface Connection {
  daemonUrl: string;
  token: string;
}

export function loadConnection(): Connection | null {
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
}

export interface Issue {
  id: string;
  title: string;
  description?: string;
  persona?: string;
  status: 'backlog' | 'todo' | 'in_progress' | 'in_review' | 'done' | 'rejected';
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

export interface IssueDetail {
  issue: Issue;
  runs: unknown[];
  proposal: Proposal | null;
}

export interface ApproveResult {
  id: string;
  status: 'done' | 'rejected';
  ability?: string;
  updated_at: string;
}

export const api = {
  health: (c: Connection) => request<Health>(c, '/v1/health'),
  agents: (c: Connection) => request<{ agents: Persona[] }>(c, '/v1/agents'),
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
};
