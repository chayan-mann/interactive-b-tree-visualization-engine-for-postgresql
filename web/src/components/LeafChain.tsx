import type { NodeView, Snapshot } from '../types';

interface Props {
  snapshot: Snapshot;
  highlight: Set<number>;
}

export function LeafChain({ snapshot, highlight }: Props) {
  if (!snapshot || !snapshot.leafChain.length) return null;
  const byId = new Map<number, NodeView>();
  snapshot.nodes.forEach((n) => byId.set(n.pageId, n));
  const leaves = snapshot.leafChain.map((id) => byId.get(id)).filter(Boolean) as NodeView[];

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
      {leaves.map((leaf, i) => (
        <div key={leaf.pageId} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <div
            style={{
              padding: '6px 10px',
              borderRadius: 6,
              background: '#2c3a5c',
              border: `1.5px solid ${highlight.has(leaf.pageId) ? '#6ea8ff' : '#4a5b7a'}`,
              fontFamily: 'JetBrains Mono, monospace',
              fontSize: 12,
              minWidth: 50,
              textAlign: 'center',
            }}
          >
            {leaf.keys.length ? `[${leaf.keys.join(', ')}]` : '[ ]'}
            <div style={{ fontSize: 9, color: '#8892a8', marginTop: 2 }}>p{leaf.pageId}</div>
          </div>
          {i < leaves.length - 1 ? <span style={{ color: '#7ee2c1', fontSize: 16 }}>→</span> : null}
        </div>
      ))}
    </div>
  );
}
