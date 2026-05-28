import { ChangeEvent, useEffect, useMemo, useState } from 'react';
import { bptreeApi } from '../api';
import type {
  BPTreeMetrics,
  BPTreeSnapshotSummary,
  OpResponse,
  ScenarioRun,
  ScenarioStep,
  Snapshot,
  TraceEvent,
} from '../types';
import { LeafChain } from './LeafChain';
import { TraceList } from './TraceList';
import { TreeSvg } from './TreeSvg';

interface HistoryEntry {
  label: string;
  detail: string;
  trace: TraceEvent[];
  snapshot: Snapshot;
  metrics?: BPTreeMetrics;
  pre?: BPTreeSnapshotSummary;
  after?: BPTreeSnapshotSummary;
}

interface ScenarioStepDef {
  op: 'insert' | 'delete' | 'search' | 'range';
  key?: number;
  value?: string;
  lo?: number;
  hi?: number;
  label?: string;
  notes?: string;
  expect?: {
    eventTypes?: string[];
    keysPresent?: number[];
    keysMissing?: number[];
    heightDelta?: number;
  };
}

interface ScenarioDef {
  name: string;
  description: string;
  operations: ScenarioStepDef[];
  order?: number;
  resetBefore?: boolean;
}

const SCENARIOS: ScenarioDef[] = [
  {
    name: 'Insert split and promote',
    description: 'Insert keys to trigger a leaf split and parent promotion.',
    order: 4,
    resetBefore: true,
    operations: [
      { op: 'insert', key: 10, value: 'row-10', label: 'insert 10', notes: 'seed' },
      { op: 'insert', key: 20, value: 'row-20', label: 'insert 20' },
      { op: 'insert', key: 30, value: 'row-30', label: 'insert 30', notes: 'causes split on next' },
      { op: 'insert', key: 40, value: 'row-40', label: 'insert 40' },
      { op: 'insert', key: 5, value: 'row-5', label: 'insert 5', expect: { eventTypes: ['split_leaf', 'promote_key'] } },
    ],
  },
  {
    name: 'Borrow from sibling',
    description: 'Delete near a sparse interior to exercise borrow.',
    order: 4,
    resetBefore: true,
    operations: [
      { op: 'insert', key: 10, value: 'row-10', label: 'insert 10' },
      { op: 'insert', key: 20, value: 'row-20', label: 'insert 20' },
      { op: 'insert', key: 30, value: 'row-30', label: 'insert 30' },
      { op: 'insert', key: 40, value: 'row-40', label: 'insert 40' },
      { op: 'delete', key: 20, label: 'delete 20', notes: 'may borrow in sibling chain', expect: { eventTypes: ['borrow_from_left_leaf', 'borrow_from_right_leaf'] } },
    ],
  },
  {
    name: 'Merge and root contraction',
    description: 'Delete from a compact tree to trigger merge and then root contraction.',
    order: 4,
    resetBefore: true,
    operations: [
      { op: 'insert', key: 1, value: 'row-1', label: 'insert 1' },
      { op: 'insert', key: 2, value: 'row-2', label: 'insert 2' },
      { op: 'insert', key: 3, value: 'row-3', label: 'insert 3' },
      { op: 'insert', key: 4, value: 'row-4', label: 'insert 4' },
      { op: 'delete', key: 2, label: 'delete 2' },
      { op: 'delete', key: 3, label: 'delete 3' },
      { op: 'delete', key: 4, label: 'delete 4', expect: { eventTypes: ['merge_leaf', 'root_contract'], heightDelta: -1 } },
    ],
  },
  {
    name: 'Range traversal',
    description: 'Seed and issue a range scan to show leaf-link traversal.',
    order: 4,
    resetBefore: true,
    operations: [
      { op: 'insert', key: 1, value: 'row-1', label: 'insert 1' },
      { op: 'insert', key: 2, value: 'row-2', label: 'insert 2' },
      { op: 'insert', key: 3, value: 'row-3', label: 'insert 3' },
      { op: 'insert', key: 4, value: 'row-4', label: 'insert 4' },
      { op: 'insert', key: 5, value: 'row-5', label: 'insert 5' },
      { op: 'range', lo: 2, hi: 4, label: 'range 2..4', notes: 'watch leaf links', expect: { eventTypes: ['leaf_link_follow', 'range_visit_leaf'] } },
    ],
  },
];

const STORAGE_MIME = 'application/json';

function buildHighlight(events: TraceEvent[], upTo: number): { highlight: Set<number>; flash: Set<number> } {
  const highlight = new Set<number>();
  const flash = new Set<number>();
  for (let i = 0; i <= upTo && i < events.length; i++) {
    const d = events[i].details ?? {};
    const readNodes = [
      'fromNode',
      'toNode',
      'leftNode',
      'rightNode',
      'leftPageId',
      'rightPageId',
      'pageId',
      'from',
      'to',
    ];
    for (const key of readNodes) {
      const v = (d as Record<string, unknown>)[key];
      if (typeof v === 'number') {
        highlight.add(v);
      }
    }
    if (Array.isArray((d as Record<string, unknown>).nodePath)) {
      ((d as Record<string, unknown>).nodePath as number[]).forEach((id) => highlight.add(id));
    }
    if (Array.isArray((d as Record<string, unknown>).pageIds)) {
      ((d as Record<string, unknown>).pageIds as number[]).forEach((id) => highlight.add(id));
    }

    if (i === upTo) {
      const t = events[i].type;
      if (
        t.startsWith('split_') ||
        t.startsWith('merge_') ||
        t.startsWith('borrow_') ||
        t === 'promote_key' ||
        t === 'new_root' ||
        t === 'root_contract'
      ) {
        for (const key of readNodes) {
          const v = (d as Record<string, unknown>)[key];
          if (typeof v === 'number') {
            flash.add(v);
          }
        }
      }
    }
  }
  return { highlight, flash };
}

function summarizeScenario(
  scenario: ScenarioDef,
  result: ScenarioRun | undefined,
  response: OpResponse,
): string[] {
  if (!result || !result.steps?.length) {
    return [`Scenario completed with no machine-readable steps`];
  }
  const summary: string[] = [];
  for (let i = 0; i < scenario.operations.length; i++) {
    const spec = scenario.operations[i];
    const step = result.steps[i];
    if (!step) {
      summary.push(`Step ${i + 1}: missing event group`);
      continue;
    }

    let ok = true;
    const miss: string[] = [];
    if (spec.expect?.eventTypes) {
      for (const need of spec.expect.eventTypes) {
        if (!(need in step.metrics.eventCountsByType) || step.metrics.eventCountsByType[need] === 0) {
          ok = false;
          miss.push(need);
        }
      }
    }
    if (spec.expect?.heightDelta !== undefined) {
      const preHeight = step.pre?.height ?? 0;
      const afterHeight = step.after?.height ?? response.snapshot.height;
      if (preHeight !== 0 && afterHeight !== preHeight + spec.expect.heightDelta) {
        ok = false;
        miss.push(`heightΔ=${spec.expect.heightDelta} (got ${afterHeight - preHeight})`);
      }
    }

    const expectedKeysPresent = spec.expect?.keysPresent ?? [];
    const expectedKeysMissing = spec.expect?.keysMissing ?? [];
    if (expectedKeysPresent.length || expectedKeysMissing.length) {
      for (const key of expectedKeysPresent) {
        if (spec.op === 'search' && step.resultFound === false) {
          ok = false;
          miss.push(`search missing ${key}`);
          continue;
        }
      }
      for (const key of expectedKeysMissing) {
        if (spec.op === 'search' && step.resultFound === true) {
          ok = false;
          miss.push(`search found ${key}`);
        } else if (spec.op === 'delete' && step.resultFound === true) {
          ok = false;
          miss.push(`delete still found ${key}`);
        }
      }
    }

    if (ok) {
      summary.push(`Step ${i + 1}: ${step.label || step.op} ✓ (${step.op})`);
    } else {
      summary.push(`Step ${i + 1}: ${step.label || step.op} ⚠ missing ${miss.join(', ')}`);
    }
  }
  return summary;
}

function checkInvariants(snapshot: Snapshot | null) {
  if (!snapshot) {
    return [{ key: 'snapshot', pass: false, msg: 'No snapshot available' }];
  }
  if (!snapshot.nodes.length) {
    return [{ key: 'snapshot', pass: false, msg: 'No nodes available' }];
  }

  const nodeById = new Map<number, { isLeaf: boolean; keys: number[]; level: number; nextPageId?: number }>();
  snapshot.nodes.forEach((n) => {
    nodeById.set(n.pageId, {
      isLeaf: n.isLeaf,
      keys: n.keys,
      level: n.level,
      nextPageId: n.nextPageId,
    });
  });

  const root = nodeById.get(snapshot.rootPageId);
  const checks: { key: string; pass: boolean; msg: string }[] = [];

  checks.push({
    key: 'root',
    pass: root != null,
    msg: root ? 'Root exists' : 'Root missing',
  });

  if (!root) {
    return checks;
  }

  checks.push({
    key: 'root-order',
    pass: root.isLeaf || root.keys.length > 0 || snapshot.height === 1,
    msg: root.isLeaf || root.keys.length > 0 || snapshot.height === 1
      ? 'Root key count valid'
      : 'Root has no keys on multi-level tree',
  });

  checks.push({
    key: 'height',
    pass: snapshot.height >= 1,
    msg: `Height is ${snapshot.height}`,
  });

  for (const node of snapshot.nodes) {
    const validMax = node.keys.length <= snapshot.maxKeys;
    const minRule = node.pageId === snapshot.rootPageId ? true : node.keys.length >= snapshot.minKeys;
    checks.push({
      key: `node-${node.pageId}-bounds`,
      pass: validMax && (node.isLeaf || minRule),
      msg: `${node.isLeaf ? 'leaf' : 'internal'} p${node.pageId} has ${node.keys.length} keys`,
    });
  }

  const leafs = snapshot.leafChain
    .map((id) => nodeById.get(id))
    .filter((n): n is NonNullable<typeof n> => n !== undefined);

  if (leafs.length === 0) {
    checks.push({ key: 'leaf-chain', pass: false, msg: 'Leaf chain is empty' });
  } else {
    let ok = true;
    for (let i = 0; i < leafs.length - 1; i++) {
      if (leafs[i].nextPageId !== snapshot.leafChain[i + 1]) {
        ok = false;
        break;
      }
    }
    checks.push({
      key: 'leaf-chain',
      pass: ok,
      msg: ok ? 'Leaf chain is linked correctly' : 'Leaf chain linkage broken',
    });

    let sorted = true;
    const chainKeys: number[] = [];
    for (const leaf of leafs) {
      chainKeys.push(...leaf.keys);
    }
    for (let i = 1; i < chainKeys.length; i++) {
      if (chainKeys[i - 1] > chainKeys[i]) {
        sorted = false;
        break;
      }
    }
    checks.push({
      key: 'leaf-chain-sort',
      pass: sorted,
      msg: sorted ? 'Leaf chain keys are ascending' : 'Leaf chain keys are unsorted',
    });
  }

  return checks;
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
  const [autoPlay, setAutoPlay] = useState(false);
  const [playSpeed, setPlaySpeed] = useState(700);
  const [focusMode, setFocusMode] = useState(false);
  const [guidedMode, setGuidedMode] = useState(true);
  const [scenarioRun, setScenarioRun] = useState<ScenarioRun | null>(null);
  const [scenarioSummary, setScenarioSummary] = useState<string[]>([]);
  const [replayFileName, setReplayFileName] = useState('');

  useEffect(() => {
    bptreeApi.snapshot().then(setSnapshot).catch((e) => setError(String(e)));
  }, []);

  const apply = async (
    label: string,
    detail: string,
    promise: Promise<OpResponse>,
    opts?: { skipHistory?: boolean },
  ) => {
    setBusy(true);
    setError(null);
    try {
      const res = await promise;
      setSnapshot(res.snapshot);
      setTrace(res.trace);
      setActiveStep(Math.max(0, res.trace.length - 1));
      let resultDetail = detail;
      if (res.found != null) {
        resultDetail += ` → ${res.found ? 'found' : 'not found'}`;
      }
      if (res.value && res.found) {
        resultDetail += ` ("${res.value}")`;
      }
      if (res.results) {
        resultDetail += ` → ${res.results.length} rows`;
      }
      if (res.metrics) {
        resultDetail += ` · writes ${res.metrics.nodeWrites}, reads ${res.metrics.nodeReads}`;
      }

      if (res.scenarioRun) {
        setScenarioRun(res.scenarioRun);
      }

      if (opts?.skipHistory) {
        return;
      }

      setHistory((h) => [
        {
          label,
          detail: resultDetail,
          trace: res.trace,
          snapshot: res.snapshot,
          metrics: res.metrics,
          pre: res.pre,
          after: res.after,
        },
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

  const runScenario = async (scenario: ScenarioDef) => {
    const ops = scenario.operations.map((op) => ({
      op: op.op,
      key: op.key,
      value: op.value,
      lo: op.lo,
      hi: op.hi,
      label: op.label,
      notes: op.notes,
    }));

    const res = await bptreeApi.bulk([], scenario.resetBefore !== false, scenario.order ?? order, ops, scenario.name);
    await apply('scenario', scenario.name, Promise.resolve(res), { skipHistory: true });
    setSnapshot(res.snapshot);
    setTrace(res.trace);
    setActiveStep(Math.max(0, res.trace.length - 1));
    setScenarioRun(res.scenarioRun ?? null);
    setScenarioSummary(summarizeScenario(scenario, res.scenarioRun, res));
    setHistory((h) => [
      {
        label: scenario.name,
        detail: `scenario: ${scenario.description}`,
        trace: res.trace,
        snapshot: res.snapshot,
        metrics: res.metrics,
        pre: res.pre,
        after: res.after,
      },
      ...h.slice(0, 49),
    ]);
  };

  const saveSession = () => {
    const payload = {
      createdAt: new Date().toISOString(),
      version: 1,
      order,
      snapshot,
      history,
      scenario: scenarioRun
        ? {
          name: scenarioRun.name,
          steps: scenarioRun.steps,
          totalSteps: scenarioRun.totalSteps,
          timestampMs: scenarioRun.timestampMs,
        }
        : null,
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: STORAGE_MIME });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `bptree-session-${Date.now()}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const onReplayFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setReplayFileName(file.name);
    try {
      const raw = JSON.parse(await file.text());
      if (Array.isArray(raw.history) && raw.history.length > 0) {
        setHistory(raw.history as HistoryEntry[]);
        const latest = raw.history[0] as HistoryEntry;
        if (latest?.snapshot) {
          setSnapshot(latest.snapshot);
          setTrace(latest.trace ?? []);
          setActiveStep(Math.max(0, (latest.trace ?? []).length - 1));
          setScenarioRun(raw.scenario ?? null);
          return;
        }
      }
      setScenarioRun(raw.scenario ?? null);
      throw new Error('Missing history in replay file');
    } catch (e) {
      setError(String(e));
    }
  };

  const { highlight, flash } = useMemo(() => buildHighlight(trace, activeStep), [trace, activeStep]);

  const invariants = useMemo(() => checkInvariants(snapshot), [snapshot]);

  useEffect(() => {
    if (!autoPlay || !trace.length) return;
    const id = window.setInterval(() => {
      setActiveStep((prev) => {
        if (prev >= trace.length - 1) {
          return prev;
        }
        return prev + 1;
      });
    }, playSpeed);
    return () => {
      window.clearInterval(id);
    };
  }, [autoPlay, playSpeed, trace.length]);

  useEffect(() => {
    if (autoPlay && trace.length > 0 && activeStep >= trace.length - 1) {
      setAutoPlay(false);
    }
  }, [autoPlay, activeStep, trace.length]);

  return (
    <div className="col" style={{ gap: 16 }}>
      <div className="panel">
        <div className="panel-title">B+ tree controls</div>
        <div className="row" style={{ gap: 16, flexWrap: 'wrap' }}>
          <div className="row" style={{ gap: 6, alignItems: 'center' }}>
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
          <div className="row" style={{ gap: 8, alignItems: 'center' }}>
            <label className="muted" style={{ fontSize: 12 }}>mode:</label>
            <button className={guidedMode ? 'primary' : ''} onClick={() => setGuidedMode((s) => !s)}>
              {guidedMode ? 'Guided ready' : 'Manual ready'}
            </button>
            <label className="row" style={{ gap: 4, fontSize: 12 }}>
              <input type="checkbox" checked={focusMode} onChange={(e) => setFocusMode(e.target.checked)} /> focus mode
            </label>
            <label className="row" style={{ gap: 4, fontSize: 12 }}>
              <input
                type="checkbox"
                checked={autoPlay}
                onChange={(e) => setAutoPlay(e.target.checked)}
              /> auto-play
            </label>
            <select
              value={playSpeed}
              onChange={(e) => setPlaySpeed(Number(e.target.value))}
              style={{ width: 110 }}
            >
              <option value="350">fast</option>
              <option value="700">medium</option>
              <option value="1100">slow</option>
            </select>
          </div>
          <div className="row" style={{ gap: 6 }}>
            <button onClick={saveSession}>Download session</button>
            <label className="row" style={{ gap: 6, border: '1px solid #2a3142', padding: '5px 8px', borderRadius: 6 }}>
              Replay from JSON
              <input type="file" accept="application/json" style={{ display: 'none' }} onChange={onReplayFile} />
            </label>
            <span className="muted" style={{ fontSize: 12 }}>{replayFileName || ''}</span>
          </div>
        </div>
        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '12px 0' }} />

        <div className="row" style={{ gap: 14, flexWrap: 'wrap' }}>
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

      {guidedMode ? (
        <div className="panel">
          <div className="panel-title">Guided scenarios</div>
          <div style={{ display: 'grid', gap: 8 }}>
            {SCENARIOS.map((scenario) => (
              <button
                key={scenario.name}
                onClick={() => void runScenario(scenario)}
                disabled={busy}
              >
                {scenario.name}
              </button>
            ))}
          </div>
          {scenarioSummary.length ? (
            <div style={{ marginTop: 10 }}>
              <div className="panel-title" style={{ marginBottom: 6 }}>Scenario checks</div>
              <ul style={{ margin: 0, paddingLeft: 18 }}>
                {scenarioSummary.map((s, i) => (
                  <li key={i} style={{ fontSize: 12, marginBottom: 4 }}>
                    {s}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      ) : null}

      {error ? (
        <div className="panel" style={{ borderColor: '#5a2828', color: '#f06e6e' }}>{error}</div>
      ) : null}

      {snapshot ? (
        <div className="panel">
          <div className="panel-title row" style={{ justifyContent: 'space-between' }}>
            <span>Tree</span>
            <span className="muted" style={{ fontSize: 11, textTransform: 'none' }}>
              order={snapshot.order} · size={snapshot.size} · height={snapshot.height} · disk reads {snapshot.diskReads} · writes {snapshot.diskWrites}
            </span>
          </div>
          <TreeSvg snapshot={snapshot} highlight={highlight} flash={flash} focusMode={focusMode} />
          <div style={{ marginTop: 14 }}>
            <div className="panel-title">Leaf chain</div>
            <LeafChain snapshot={snapshot} highlight={highlight} focusMode={focusMode} />
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
            <div className="row" style={{ marginTop: 10, gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <button
                onClick={() =>
                  setActiveStep((s) => {
                    const next = Math.max(0, s - 1);
                    return next;
                  })
                }
                disabled={activeStep === 0}
              >
                ◀ Prev
              </button>
              <button
                onClick={() =>
                  setActiveStep((s) => Math.min(trace.length - 1, s + 1))
                }
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
                  {h.metrics ? (
                    <div className="muted" style={{ fontSize: 11 }}>
                      reads {h.metrics.nodeReads} · writes {h.metrics.nodeWrites}
                    </div>
                  ) : null}
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      <div className="panel">
        <div className="panel-title">Invariant checklist</div>
        <ul style={{ margin: 0, paddingLeft: 18 }}>
          {invariants.map((v) => (
            <li key={v.key} style={{ fontSize: 12, color: v.pass ? '#66c27a' : '#f06e6e', marginBottom: 4 }}>
              {v.pass ? '✓ ' : '⚠ '}
              {v.msg}
            </li>
          ))}
        </ul>
      </div>

      {scenarioRun ? (
        <div className="panel">
          <div className="panel-title">Scenario trace groups</div>
          {scenarioRun.steps.map((step: ScenarioStep) => (
            <div key={`${step.step}-${step.op}`} style={{ marginBottom: 6 }}>
              <div className="code" style={{ fontSize: 12 }}>
                #{step.step} {step.label || step.op} ({step.metrics?.pathLength ?? 0} hops)
              </div>
              <div className="muted" style={{ fontSize: 12 }}>
                events {step.eventFrom}-{step.eventTo} · writes {step.metrics?.nodeWrites ?? 0} · reads {step.metrics?.nodeReads ?? 0}
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
