import type { TraceEvent } from '../types';

interface Props {
  trace: TraceEvent[];
  activeIdx: number;
  onJump: (idx: number) => void;
}

const FRIENDLY: Record<string, string> = {
  insert_start: 'Begin insert',
  insert_into_leaf: 'Inserted into leaf',
  insert_update: 'Updated existing key',
  split_leaf: 'Split leaf',
  split_internal: 'Split internal node',
  promote_key: 'Promoted separator to parent',
  new_root: 'Created new root',
  delete_start: 'Begin delete',
  delete_from_leaf: 'Removed from leaf',
  delete_miss: 'Key not found',
  borrow_from_left_leaf: 'Borrowed from left sibling (leaf)',
  borrow_from_right_leaf: 'Borrowed from right sibling (leaf)',
  borrow_from_left_internal: 'Borrowed from left sibling (internal)',
  borrow_from_right_internal: 'Borrowed from right sibling (internal)',
  borrow_from_left_leaf_done: 'Left borrow completed',
  borrow_from_right_leaf_done: 'Right borrow completed',
  merge_leaf: 'Merged leaves',
  merge_internal: 'Merged internal nodes',
  root_contract: 'Root contracted',
  search_start: 'Begin search',
  search_hit: 'Search hit',
  search_miss: 'Search miss',
  range_start: 'Begin range scan',
  range_visit_leaf: 'Scanned leaf',
  leaf_link_follow: 'Followed leaf link',
  range_end: 'Range scan complete',
  path: 'Traversed path',
};

export function TraceList({ trace, activeIdx, onJump }: Props) {
  if (!trace.length) {
    return <div className="muted">No events for this operation.</div>;
  }
  return (
    <ol style={{ listStyle: 'none', padding: 0, margin: 0, maxHeight: 280, overflow: 'auto' }}>
      {trace.map((ev, i) => {
        const friendly = FRIENDLY[ev.type] ?? ev.type;
        const active = i === activeIdx;
        return (
          <li
            key={`${ev.operationId ?? 'op'}-${i}`}
            onClick={() => onJump(i)}
            style={{
              padding: '6px 8px',
              borderRadius: 6,
              cursor: 'pointer',
              background: active ? '#2c3a5c' : 'transparent',
              borderLeft: active ? '3px solid #6ea8ff' : '3px solid transparent',
              fontSize: 12,
              marginBottom: 2,
            }}
          >
            <div style={{ fontWeight: 600 }}>
              {ev.label ? `${ev.label}: ` : ''}{friendly}
            </div>
            {ev.notes ? <div className="muted code">{ev.notes}</div> : null}
            {ev.operationId ? <div className="muted code">id: {ev.operationId}</div> : null}
            {ev.details ? <div className="code muted">{summarize(ev)}</div> : null}
            {ev.eventPhase ? <div className="muted" style={{ fontSize: 11 }}>phase: {ev.eventPhase}</div> : null}
          </li>
        );
      })}
    </ol>
  );
}

function summarize(ev: TraceEvent): string {
  const d = ev.details ?? {};
  const bits: string[] = [];
  if ('key' in d) bits.push(`key=${d.key}`);
  if ('fromNode' in d) bits.push(`from p${d.fromNode}`);
  if ('toNode' in d) bits.push(`to p${d.toNode}`);
  if ('separator' in d) bits.push(`separator=${d.separator}`);
  if ('pageId' in d) bits.push(`page ${d.pageId}`);
  if ('leftPageId' in d) bits.push(`left p${d.leftPageId}`);
  if ('rightPageId' in d) bits.push(`right p${d.rightPageId}`);
  if ('pageIds' in d && Array.isArray(d.pageIds)) bits.push(`pages [${d.pageIds.join(' → ')}]`);
  if ('nodePath' in d && Array.isArray(d.nodePath)) bits.push(`path [${d.nodePath.join(' → ')}]`);
  if ('rebalanceResult' in d) bits.push(`${d.rebalanceResult}`);
  if ('keys' in d && Array.isArray(d.keys)) bits.push(`keys [${d.keys.join(', ')}]`);
  if ('nodeKeys' in d && Array.isArray(d.nodeKeys)) bits.push(`node ${d.nodeKeys.join(', ')}`);
  if ('from' in d && 'to' in d) bits.push(`p${d.from} → p${d.to}`);
  if ('lo' in d && 'hi' in d) bits.push(`[${d.lo}..${d.hi}]`);
  if ('count' in d) bits.push(`${d.count} rows`);
  if ('value' in d && typeof d.value === 'string' && d.value) bits.push(`value="${d.value}"`);
  return bits.join('  ');
}
