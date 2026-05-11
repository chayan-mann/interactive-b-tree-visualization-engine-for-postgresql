import type { NodeView, Snapshot } from '../types';

interface Props {
  snapshot: Snapshot;
  highlight: Set<number>;
  flash: Set<number>;
}

interface Positioned extends NodeView {
  x: number;
  y: number;
  width: number;
}

const NODE_HEIGHT = 38;
const SLOT_WIDTH = 38;
const MIN_NODE_WIDTH = 60;
const H_GAP = 24;
const V_GAP = 70;

function measure(node: NodeView): number {
  const slots = Math.max(node.keys.length, 1);
  return Math.max(MIN_NODE_WIDTH, slots * SLOT_WIDTH + 8);
}

export function TreeSvg({ snapshot, highlight, flash }: Props) {
  if (!snapshot || !snapshot.nodes.length) {
    return <div className="muted">empty tree</div>;
  }

  const byId = new Map<number, NodeView>();
  snapshot.nodes.forEach((n) => byId.set(n.pageId, n));

  // Layout: classic tidy-tree using a simple recursive method that lays out
  // leaves first, then centers each parent above its children.
  const positioned = new Map<number, Positioned>();
  const leafOrderIndex = new Map<number, number>();
  snapshot.leafChain.forEach((id, i) => leafOrderIndex.set(id, i));

  function layout(nodeId: number, depth: number): { x: number; width: number } {
    const node = byId.get(nodeId)!;
    const w = measure(node);
    if (node.isLeaf || !node.childPageIds || node.childPageIds.length === 0) {
      const idx = leafOrderIndex.get(node.pageId) ?? 0;
      const x = idx * (MIN_NODE_WIDTH + H_GAP);
      positioned.set(node.pageId, { ...node, x, y: depth * V_GAP, width: w });
      return { x, width: w };
    }
    const childResults = node.childPageIds.map((cid) => layout(cid, depth + 1));
    const first = childResults[0];
    const last = childResults[childResults.length - 1];
    const centerX = (first.x + last.x + last.width) / 2 - w / 2;
    positioned.set(node.pageId, {
      ...node,
      x: centerX,
      y: depth * V_GAP,
      width: w,
    });
    return { x: centerX, width: w };
  }
  layout(snapshot.rootPageId, 0);

  let minX = Infinity;
  let maxX = -Infinity;
  let maxY = 0;
  positioned.forEach((n) => {
    if (n.x < minX) minX = n.x;
    if (n.x + n.width > maxX) maxX = n.x + n.width;
    if (n.y + NODE_HEIGHT > maxY) maxY = n.y + NODE_HEIGHT;
  });
  const padX = 30;
  const padY = 20;
  const width = maxX - minX + padX * 2;
  const height = maxY + padY * 2;

  const edges: { x1: number; y1: number; x2: number; y2: number; key: string }[] = [];
  positioned.forEach((n) => {
    if (n.isLeaf || !n.childPageIds) return;
    n.childPageIds.forEach((cid) => {
      const c = positioned.get(cid);
      if (!c) return;
      edges.push({
        x1: n.x + n.width / 2 - minX + padX,
        y1: n.y + NODE_HEIGHT - minX * 0 + padY,
        x2: c.x + c.width / 2 - minX + padX,
        y2: c.y + padY,
        key: `${n.pageId}-${cid}`,
      });
    });
  });

  const leafLinks: { x1: number; y1: number; x2: number; y2: number; key: string }[] = [];
  positioned.forEach((n) => {
    if (!n.isLeaf || n.nextPageId == null) return;
    const next = positioned.get(n.nextPageId);
    if (!next) return;
    leafLinks.push({
      x1: n.x + n.width - minX + padX,
      y1: n.y + NODE_HEIGHT / 2 + padY,
      x2: next.x - minX + padX,
      y2: next.y + NODE_HEIGHT / 2 + padY,
      key: `leaf-${n.pageId}`,
    });
  });

  return (
    <div style={{ overflow: 'auto', maxWidth: '100%' }}>
      <svg
        width={Math.max(width, 400)}
        height={Math.max(height, 120)}
        style={{ display: 'block' }}
      >
        <defs>
          <marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="3"
            orient="auto" markerUnits="strokeWidth">
            <path d="M0,0 L0,6 L6,3 z" fill="#7ee2c1" />
          </marker>
        </defs>
        {edges.map((e) => (
          <line
            key={e.key}
            x1={e.x1}
            y1={e.y1}
            x2={e.x2}
            y2={e.y2}
            stroke="#3a4256"
            strokeWidth={1.5}
          />
        ))}
        {leafLinks.map((e) => (
          <line
            key={e.key}
            x1={e.x1}
            y1={e.y1}
            x2={e.x2}
            y2={e.y2}
            stroke="#7ee2c1"
            strokeWidth={1.5}
            strokeDasharray="4 4"
            markerEnd="url(#arrow)"
          />
        ))}
        {Array.from(positioned.values()).map((n) => {
          const isHighlight = highlight.has(n.pageId);
          const isFlash = flash.has(n.pageId);
          const fill = n.isLeaf ? '#2c3a5c' : '#2c3a3a';
          const stroke = isFlash ? '#f0b86e' : isHighlight ? '#6ea8ff' : '#4a5b7a';
          const strokeWidth = isFlash || isHighlight ? 2.5 : 1.2;
          return (
            <g key={n.pageId} transform={`translate(${n.x - minX + padX}, ${n.y + padY})`}>
              <rect
                width={n.width}
                height={NODE_HEIGHT}
                rx={6}
                ry={6}
                fill={fill}
                stroke={stroke}
                strokeWidth={strokeWidth}
              />
              <text
                x={n.width / 2}
                y={NODE_HEIGHT / 2 + 4}
                textAnchor="middle"
                fontFamily="JetBrains Mono, monospace"
                fontSize={12}
                fill="#e4e8f1"
              >
                {n.keys.length ? n.keys.join(' • ') : '∅'}
              </text>
              <text
                x={n.width / 2}
                y={-4}
                textAnchor="middle"
                fontFamily="Inter, sans-serif"
                fontSize={9}
                fill="#8892a8"
              >
                p{n.pageId}
                {n.isLeaf ? ' (leaf)' : ''}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}
