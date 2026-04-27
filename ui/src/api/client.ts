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
  created_at: string;
  updated_at: string;
}

export interface Proposal {
  type: string;
  content: string;
  target?: Record<string, unknown>;
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
  approve: (c: Connection, id: string) =>
    request<ApproveResult>(c, `/v1/issues/${id}/approve`, { method: 'POST' }),
  reject: (c: Connection, id: string) =>
    request<ApproveResult>(c, `/v1/issues/${id}/reject`, { method: 'POST' }),
};
