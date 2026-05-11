import { useEffect, useMemo, useState } from 'react';
import { bptreeApi } from '../api';
import type { OpResponse, Snapshot, TraceEvent } from '../types';
import { LeafChain } from './LeafChain';
import { TraceList } from './TraceList';
import { TreeSvg } from './TreeSvg';

interface HistoryEntry {
  label: string;
  detail: string;
  trace: TraceEvent[];
  snapshot: Snapshot;
}

function buildHighlight(events: TraceEvent[], upTo: number): { highlight: Set<number>; flash: Set<number> } {
  const highlight = new Set<number>();
  const flash = new Set<number>();
  for (let i = 0; i <= upTo && i < events.length; i++) {
    const d = events[i].details ?? {};
    if (Array.isArray((d as Record<string, unknown>).pageIds)) {
      ((d as Record<string, unknown>).pageIds as number[]).forEach((id) => highlight.add(id));
    }
    if (typeof (d as Record<string, unknown>).pageId === 'number') {
      highlight.add((d as Record<string, unknown>).pageId as number);
    }
    if (typeof (d as Record<string, unknown>).leftPageId === 'number')
      highlight.add((d as Record<string, unknown>).leftPageId as number);
    if (typeof (d as Record<string, unknown>).rightPageId === 'number')
      highlight.add((d as Record<string, unknown>).rightPageId as number);
    if (i === upTo) {
      const last = events[i];
      if (
        last.type.startsWith('split_') ||
        last.type.startsWith('merge_') ||
        last.type.startsWith('borrow_') ||
        last.type === 'promote_key' ||
        last.type === 'new_root' ||
        last.type === 'root_contract'
      ) {
        if (typeof (d as Record<string, unknown>).pageId === 'number')
          flash.add((d as Record<string, unknown>).pageId as number);
        if (typeof (d as Record<string, unknown>).leftPageId === 'number')
          flash.add((d as Record<string, unknown>).leftPageId as number);
        if (typeof (d as Record<string, unknown>).rightPageId === 'number')
          flash.add((d as Record<string, unknown>).rightPageId as number);
      }
    }
  }
  return { highlight, flash };
}

export function BPTreePanel() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [order, setOrder] = useState(4);
  const [insertKey, setInsertKey] = useState('');
  const [insertValue, setInsertValue] = useState('');
  const [deleteKey, setDeleteKey] = useState('');
  const [searchKey, setSearchKey] = useState('');
  const [rangeLo, setRangeLo] = useState('');
  const [rangeHi, setRangeHi] = useState('');
  const [trace, setTrace] = useState<TraceEvent[]>([]);
  const [activeStep, setActiveStep] = useState(0);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    bptreeApi.snapshot().then(setSnapshot).catch((e) => setError(String(e)));
  }, []);

  const apply = async (
    label: string,
    detail: string,
    promise: Promise<OpResponse>,
  ) => {
    setBusy(true);
    setError(null);
    try {
      const res = await promise;
      setSnapshot(res.snapshot);
      setTrace(res.trace);
      setActiveStep(Math.max(0, res.trace.length - 1));
      let resultDetail = detail;
      if (res.found != null) resultDetail += ` → ${res.found ? 'found' : 'not found'}`;
      if (res.value && res.found) resultDetail += ` ("${res.value}")`;
      if (res.results) resultDetail += ` → ${res.results.length} rows`;
      setHistory((h) => [
        { label, detail: resultDetail, trace: res.trace, snapshot: res.snapshot },
        ...h.slice(0, 49),
      ]);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const onReset = () => apply('reset', `order=${order}`, bptreeApi.reset(order));
  const onInsert = () => {
    const k = Number(insertKey);
    if (!Number.isFinite(k)) return;
    const v = insertValue || `row-${k}`;
    setInsertKey('');
    setInsertValue('');
    void apply('insert', `${k} = "${v}"`, bptreeApi.insert(k, v));
  };
  const onDelete = () => {
    const k = Number(deleteKey);
    if (!Number.isFinite(k)) return;
    setDeleteKey('');
    void apply('delete', `${k}`, bptreeApi.delete(k));
  };
  const onSearch = () => {
    const k = Number(searchKey);
    if (!Number.isFinite(k)) return;
    setSearchKey('');
    void apply('search', `${k}`, bptreeApi.search(k));
  };
  const onRange = () => {
    const lo = Number(rangeLo);
    const hi = Number(rangeHi);
    if (!Number.isFinite(lo) || !Number.isFinite(hi)) return;
    setRangeLo('');
    setRangeHi('');
    void apply('range', `${lo}..${hi}`, bptreeApi.range(lo, hi));
  };

  const onSeedRandom = () => {
    const keys: number[] = [];
    const seen = new Set<number>();
    while (keys.length < 20) {
      const k = Math.floor(Math.random() * 200) + 1;
      if (!seen.has(k)) {
        seen.add(k);
        keys.push(k);
      }
    }
    void apply('bulk', `seed ${keys.length} keys`, bptreeApi.bulk(keys, true, order));
  };

  const { highlight, flash } = useMemo(
    () => buildHighlight(trace, activeStep),
    [trace, activeStep],
  );

  return (
    <div className="col" style={{ gap: 16 }}>
      <div className="panel">
        <div className="panel-title">B+ tree controls</div>
        <div className="row" style={{ gap: 16 }}>
          <div className="row" style={{ gap: 6 }}>
            <label className="muted" style={{ fontSize: 12 }}>order</label>
            <input
              type="number"
              min={3}
              max={20}
              value={order}
              onChange={(e) => setOrder(Number(e.target.value))}
              style={{ width: 60 }}
            />
            <button onClick={onReset} disabled={busy}>Reset</button>
            <button onClick={onSeedRandom} disabled={busy}>Seed 20 random</button>
          </div>
        </div>
        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '12px 0' }} />
        <div className="row" style={{ gap: 14 }}>
          <div className="row" style={{ gap: 6 }}>
            <input
              placeholder="key"
              value={insertKey}
              onChange={(e) => setInsertKey(e.target.value)}
              style={{ width: 80 }}
            />
            <input
              placeholder="value (optional)"
              value={insertValue}
              onChange={(e) => setInsertValue(e.target.value)}
              style={{ width: 140 }}
            />
            <button className="primary" onClick={onInsert} disabled={busy}>Insert</button>
          </div>
          <div className="row" style={{ gap: 6 }}>
            <input
              placeholder="key"
              value={deleteKey}
              onChange={(e) => setDeleteKey(e.target.value)}
              style={{ width: 80 }}
            />
            <button className="danger" onClick={onDelete} disabled={busy}>Delete</button>
          </div>
          <div className="row" style={{ gap: 6 }}>
            <input
              placeholder="key"
              value={searchKey}
              onChange={(e) => setSearchKey(e.target.value)}
              style={{ width: 80 }}
            />
            <button onClick={onSearch} disabled={busy}>Search</button>
          </div>
          <div className="row" style={{ gap: 6 }}>
            <input
              placeholder="lo"
              value={rangeLo}
              onChange={(e) => setRangeLo(e.target.value)}
              style={{ width: 60 }}
            />
            <input
              placeholder="hi"
              value={rangeHi}
              onChange={(e) => setRangeHi(e.target.value)}
              style={{ width: 60 }}
            />
            <button onClick={onRange} disabled={busy}>Range</button>
          </div>
        </div>
      </div>

      {error ? (
        <div className="panel" style={{ borderColor: '#5a2828', color: '#f06e6e' }}>{error}</div>
      ) : null}

      {snapshot ? (
        <div className="panel">
          <div className="panel-title row" style={{ justifyContent: 'space-between' }}>
            <span>Tree</span>
            <span className="muted" style={{ fontSize: 11, textTransform: 'none' }}>
              order={snapshot.order} · size={snapshot.size} · height={snapshot.height} · disk
              reads {snapshot.diskReads} · writes {snapshot.diskWrites}
            </span>
          </div>
          <TreeSvg snapshot={snapshot} highlight={highlight} flash={flash} />
          <div style={{ marginTop: 14 }}>
            <div className="panel-title">Leaf chain</div>
            <LeafChain snapshot={snapshot} highlight={highlight} />
          </div>
        </div>
      ) : null}

      <div className="row" style={{ alignItems: 'stretch', gap: 16 }}>
        <div className="panel" style={{ flex: 1, minWidth: 280 }}>
          <div className="panel-title row" style={{ justifyContent: 'space-between' }}>
            <span>Trace</span>
            <span className="muted" style={{ fontSize: 11, textTransform: 'none' }}>
              click an event to highlight the affected pages
            </span>
          </div>
          <TraceList trace={trace} activeIdx={activeStep} onJump={setActiveStep} />
          {trace.length ? (
            <div className="row" style={{ marginTop: 10 }}>
              <button
                onClick={() => setActiveStep((s) => Math.max(0, s - 1))}
                disabled={activeStep === 0}
              >
                ◀ Prev
              </button>
              <button
                onClick={() => setActiveStep((s) => Math.min(trace.length - 1, s + 1))}
                disabled={activeStep >= trace.length - 1}
              >
                Next ▶
              </button>
              <span className="muted" style={{ fontSize: 12 }}>
                Step {activeStep + 1} / {trace.length}
              </span>
            </div>
          ) : null}
        </div>

        <div className="panel" style={{ flex: 1, minWidth: 280 }}>
          <div className="panel-title">Operation history</div>
          <div style={{ maxHeight: 320, overflow: 'auto' }}>
            {history.length === 0 ? (
              <div className="muted">No operations yet.</div>
            ) : (
              history.map((h, i) => (
                <div
                  key={i}
                  onClick={() => {
                    setSnapshot(h.snapshot);
                    setTrace(h.trace);
                    setActiveStep(Math.max(0, h.trace.length - 1));
                  }}
                  style={{
                    padding: '6px 8px',
                    borderRadius: 6,
                    cursor: 'pointer',
                    fontSize: 12,
                    marginBottom: 2,
                    background: '#1d2230',
                  }}
                >
                  <span className="tag blue">{h.label}</span>
                  <span className="code">{h.detail}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
