import { useCallback, useEffect, useState } from 'react';
import { pgApi } from '../api';
import type { CompareResponse, ExplainResponse, PgQueryResult, PgStatus } from '../api';
import type { Recommendation } from '../types';
import { PlanView } from './PlanView';

const SAMPLE_QUERIES = [
  {
    label: 'Equality scan on age',
    sql: 'SELECT * FROM users_demo WHERE age = 30;',
  },
  {
    label: 'Range scan on age',
    sql: 'SELECT * FROM users_demo WHERE age BETWEEN 20 AND 30;',
  },
  {
    label: 'Composite predicate',
    sql: "SELECT * FROM users_demo WHERE city = 'Mumbai' AND age = 25;",
  },
  {
    label: 'Index-only candidate',
    sql: 'SELECT age, username FROM users_demo WHERE age = 27;',
  },
  {
    label: 'Time window',
    sql: "SELECT id FROM users_demo WHERE created_at > NOW() - INTERVAL '30 days';",
  },
];

interface FormState {
  name: string;
  columns: string;
  include: string;
  unique: boolean;
}

const EMPTY_FORM: FormState = { name: '', columns: '', include: '', unique: false };

export function PgLabPanel() {
  const [available, setAvailable] = useState<boolean | null>(null);
  const [status, setStatus] = useState<PgStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [seedRows, setSeedRows] = useState('100000');
  const [sql, setSql] = useState(SAMPLE_QUERIES[0].sql);
  const [queryResult, setQueryResult] = useState<PgQueryResult | null>(null);
  const [explain, setExplain] = useState<ExplainResponse | null>(null);
  const [compare, setCompare] = useState<CompareResponse | null>(null);
  const [recs, setRecs] = useState<Recommendation[]>([]);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);

  const refreshStatus = useCallback(async () => {
    try {
      const s = await pgApi.status();
      setStatus(s);
      setAvailable(true);
    } catch (e) {
      setAvailable(false);
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    refreshStatus();
  }, [refreshStatus]);

  if (available === false) {
    return (
      <div className="panel">
        <div className="panel-title">PostgreSQL lab</div>
        <p>
          The PostgreSQL lab is not configured. Set <code>INDEXLAB_DSN</code> (or pass
          <code> -dsn</code> to the server) and restart. Example:
        </p>
        <pre className="code" style={{ background: '#1d2230', padding: 8, borderRadius: 6 }}>
INDEXLAB_DSN="postgres://indexlab:indexlab@localhost:5432/indexlab?sslmode=disable" \
  go run ./cmd/server
        </pre>
        <button onClick={refreshStatus}>Retry connection</button>
      </div>
    );
  }

  const run = async <T,>(label: string, fn: () => Promise<T>, after?: (v: T) => void) => {
    setBusy(true);
    setError(null);
    try {
      const v = await fn();
      after?.(v);
      return v;
    } catch (e) {
      setError(`${label}: ${e}`);
      return null;
    } finally {
      setBusy(false);
    }
  };

  const onSetup = () => run('setup', pgApi.setup, () => refreshStatus());
  const onSeed = () =>
    run(
      'seed',
      () => pgApi.seed(Number(seedRows) || 100000, true),
      () => refreshStatus(),
    );
  const onQuery = () => run('query', () => pgApi.query(sql), setQueryResult);
  const onExplain = () => run('explain', () => pgApi.explain(sql), setExplain);
  const onRecommend = () =>
    run(
      'recommend',
      () => pgApi.recommend(sql),
      (v) => setRecs(v.recommendations ?? []),
    );
  const onCompare = () => {
    if (!form.columns.trim() || !form.name.trim()) {
      setError('compare: provide an index name and column list first');
      return;
    }
    const columns = form.columns.split(',').map((s) => s.trim()).filter(Boolean);
    const include = form.include
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    return run(
      'compare',
      () => pgApi.compare(sql, { name: form.name, columns, include }),
      setCompare,
    );
  };
  const onCreate = async () => {
    const columns = form.columns.split(',').map((s) => s.trim()).filter(Boolean);
    const include = form.include
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    await run('create', () =>
      pgApi.createIndex({ name: form.name, columns, include, unique: form.unique }),
    );
    setForm(EMPTY_FORM);
    await refreshStatus();
  };
  const onDrop = async (name: string) => {
    await run('drop', () => pgApi.dropIndex(name));
    await refreshStatus();
  };

  const applyRecommendation = (r: Recommendation) => {
    setForm({
      name: r.sql.match(/CREATE INDEX (\S+)/)?.[1] ?? 'idx',
      columns: r.columns.join(', '),
      include: '',
      unique: false,
    });
  };

  return (
    <div className="col" style={{ gap: 16 }}>
      <div className="panel">
        <div className="panel-title row" style={{ justifyContent: 'space-between' }}>
          <span>PostgreSQL lab</span>
          <span className="muted" style={{ fontSize: 11, textTransform: 'none' }}>
            {status ? `${status.rows.toLocaleString()} rows · ${status.indexes.length} indexes` : 'loading…'}
          </span>
        </div>
        <div className="row" style={{ gap: 10 }}>
          <button onClick={onSetup} disabled={busy}>Create table</button>
          <div className="row" style={{ gap: 6 }}>
            <input
              type="number"
              min={1000}
              value={seedRows}
              onChange={(e) => setSeedRows(e.target.value)}
              style={{ width: 110 }}
            />
            <button onClick={onSeed} disabled={busy}>Seed (truncate first)</button>
          </div>
          <button onClick={refreshStatus} disabled={busy}>Refresh status</button>
        </div>
        {status && status.indexes.length ? (
          <div style={{ marginTop: 12 }}>
            <div className="panel-title">Current indexes</div>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ textAlign: 'left', color: '#8892a8' }}>
                  <th style={{ padding: 4 }}>name</th>
                  <th style={{ padding: 4 }}>definition</th>
                  <th style={{ padding: 4 }}>size</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {status.indexes.map((i) => (
                  <tr key={i.name} style={{ borderTop: '1px solid #2a3142' }}>
                    <td style={{ padding: 4 }} className="code">{i.name}</td>
                    <td style={{ padding: 4 }} className="code">{i.definition}</td>
                    <td style={{ padding: 4 }} className="code">{formatBytes(i.sizeBytes)}</td>
                    <td style={{ padding: 4 }}>
                      <button onClick={() => onDrop(i.name)} disabled={busy}>Drop</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>

      <div className="panel">
        <div className="panel-title">Query & explain</div>
        <div style={{ marginBottom: 8 }}>
          {SAMPLE_QUERIES.map((q) => (
            <button
              key={q.label}
              style={{ marginRight: 6, marginBottom: 6 }}
              onClick={() => setSql(q.sql)}
            >
              {q.label}
            </button>
          ))}
        </div>
        <textarea
          rows={4}
          value={sql}
          onChange={(e) => setSql(e.target.value)}
        />
        <div className="row" style={{ marginTop: 8 }}>
          <button className="primary" onClick={onExplain} disabled={busy}>EXPLAIN ANALYZE</button>
          <button onClick={onQuery} disabled={busy}>Run query</button>
          <button onClick={onRecommend} disabled={busy}>Recommend index</button>
        </div>
      </div>

      {error ? (
        <div className="panel" style={{ borderColor: '#5a2828', color: '#f06e6e' }}>{error}</div>
      ) : null}

      {recs.length ? (
        <div className="panel">
          <div className="panel-title">Suggested indexes</div>
          {recs.map((r, i) => (
            <div key={i} style={{ marginBottom: 10 }}>
              <div className="code" style={{ marginBottom: 2 }}>{r.sql}</div>
              <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{r.reason}</div>
              <button onClick={() => applyRecommendation(r)}>Use as index spec</button>
            </div>
          ))}
        </div>
      ) : null}

      <div className="panel">
        <div className="panel-title">Create or compare index</div>
        <div className="row" style={{ gap: 8, alignItems: 'flex-end' }}>
          <div className="col" style={{ gap: 2 }}>
            <label className="muted" style={{ fontSize: 11 }}>name</label>
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="idx_users_demo_age"
              style={{ width: 220 }}
            />
          </div>
          <div className="col" style={{ gap: 2 }}>
            <label className="muted" style={{ fontSize: 11 }}>columns (comma)</label>
            <input
              value={form.columns}
              onChange={(e) => setForm({ ...form, columns: e.target.value })}
              placeholder="age"
              style={{ width: 220 }}
            />
          </div>
          <div className="col" style={{ gap: 2 }}>
            <label className="muted" style={{ fontSize: 11 }}>include (covering)</label>
            <input
              value={form.include}
              onChange={(e) => setForm({ ...form, include: e.target.value })}
              placeholder="username"
              style={{ width: 220 }}
            />
          </div>
          <label className="row" style={{ gap: 4, fontSize: 12 }}>
            <input
              type="checkbox"
              checked={form.unique}
              onChange={(e) => setForm({ ...form, unique: e.target.checked })}
            />
            unique
          </label>
          <button onClick={onCreate} disabled={busy}>Create index</button>
          <button className="primary" onClick={onCompare} disabled={busy}>
            Compare before/after
          </button>
        </div>
      </div>

      {explain ? <PlanView title="Latest EXPLAIN ANALYZE" report={explain.report} /> : null}

      {compare ? (
        <div className="panel">
          <div className="panel-title">Before / after comparison</div>
          <ul style={{ marginTop: 0 }}>
            {compare.summary.map((s, i) => (
              <li key={i} style={{ fontSize: 13 }}>{s}</li>
            ))}
          </ul>
          <div className="row" style={{ alignItems: 'stretch', gap: 16 }}>
            <PlanView title="Before index" report={compare.before} />
            <PlanView title="After index" report={compare.after} />
          </div>
        </div>
      ) : null}

      {queryResult ? (
        <div className="panel">
          <div className="panel-title row" style={{ justifyContent: 'space-between' }}>
            <span>Query results</span>
            <span className="muted" style={{ fontSize: 11, textTransform: 'none' }}>
              {queryResult.rows.length} rows{queryResult.truncated ? ' (truncated to 200)' : ''} · {(queryResult.durationNs / 1e6).toFixed(2)} ms
            </span>
          </div>
          <div style={{ overflow: 'auto', maxHeight: 320 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr>
                  {queryResult.columns.map((c) => (
                    <th key={c} style={{ textAlign: 'left', padding: 4, color: '#8892a8' }}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {queryResult.rows.map((row, i) => (
                  <tr key={i} style={{ borderTop: '1px solid #2a3142' }}>
                    {row.map((cell, j) => (
                      <td key={j} className="code" style={{ padding: 4 }}>
                        {String(cell)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}
