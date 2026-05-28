import type {
  IndexInfo,
  OpResponse,
  PlanReport,
  Recommendation,
  Snapshot,
} from './types';

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `${res.status} ${res.statusText}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const bptreeApi = {
  snapshot: () => send<Snapshot>('/api/bptree/snapshot'),
  reset: (order: number) =>
    send<OpResponse>('/api/bptree/reset', { method: 'POST', body: JSON.stringify({ order }) }),
  insert: (key: number, value: string) =>
    send<OpResponse>('/api/bptree/insert', { method: 'POST', body: JSON.stringify({ key, value }) }),
  delete: (key: number) =>
    send<OpResponse>('/api/bptree/delete', { method: 'POST', body: JSON.stringify({ key }) }),
  search: (key: number) =>
    send<OpResponse>('/api/bptree/search', { method: 'POST', body: JSON.stringify({ key }) }),
  range: (lo: number, hi: number) =>
    send<OpResponse>('/api/bptree/range', { method: 'POST', body: JSON.stringify({ lo, hi }) }),
  bulk: (
    keys: number[],
    reset: boolean,
    order: number,
    operations?: Array<{
      op: string;
      key?: number;
      value?: string;
      lo?: number;
      hi?: number;
      label?: string;
      notes?: string;
    }>,
    scenarioName?: string,
  ) =>
    send<OpResponse>('/api/bptree/bulk', {
      method: 'POST',
      body: JSON.stringify({
        keys,
        reset,
        order,
        operations,
        scenarioName,
      }),
    }),
};

export interface PgStatus {
  rows: number;
  indexes: IndexInfo[];
}

export interface PgQueryResult {
  columns: string[];
  rows: unknown[][];
  truncated: boolean;
  durationNs: number;
}

export interface ExplainResponse {
  raw: unknown;
  report: PlanReport;
}

export interface CompareResponse {
  before: PlanReport;
  after: PlanReport;
  summary: string[];
}

export const pgApi = {
  status: () => send<PgStatus>('/api/pglab/status'),
  setup: () => send<{ status: string }>('/api/pglab/setup', { method: 'POST' }),
  seed: (rows: number, truncate: boolean) =>
    send<{ status: string; rows: number }>('/api/pglab/seed', {
      method: 'POST',
      body: JSON.stringify({ rows, truncate }),
    }),
  query: (sql: string) =>
    send<PgQueryResult>('/api/pglab/query', { method: 'POST', body: JSON.stringify({ sql }) }),
  explain: (sql: string) =>
    send<ExplainResponse>('/api/pglab/explain', { method: 'POST', body: JSON.stringify({ sql }) }),
  createIndex: (spec: {
    name: string;
    table?: string;
    columns: string[];
    include?: string[];
    unique?: boolean;
  }) =>
    send<{ status: string }>('/api/pglab/index', { method: 'POST', body: JSON.stringify(spec) }),
  dropIndex: (name: string) =>
    send<{ status: string }>(`/api/pglab/index?name=${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),
  compare: (sql: string, index: { name: string; table?: string; columns: string[]; include?: string[] }) =>
    send<CompareResponse>('/api/pglab/compare', {
      method: 'POST',
      body: JSON.stringify({ sql, index }),
    }),
  recommend: (sql: string) =>
    send<{ recommendations: Recommendation[] }>('/api/pglab/recommend', {
      method: 'POST',
      body: JSON.stringify({ sql }),
    }),
};
