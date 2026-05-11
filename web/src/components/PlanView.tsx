import type { PlanNodeT, PlanReport } from '../types';

interface Props {
  title: string;
  report: PlanReport | null;
}

function PlanNode({ node, depth = 0 }: { node: PlanNodeT; depth?: number }) {
  return (
    <div style={{ marginLeft: depth * 14, paddingLeft: 10, borderLeft: depth ? '1px dashed #2a3142' : 'none' }}>
      <div style={{ display: 'flex', gap: 8, alignItems: 'baseline', marginBottom: 2 }}>
        <span className="tag blue">{node.nodeType}</span>
        {node.relation ? <span className="code">on {node.relation}</span> : null}
        {node.indexName ? <span className="code muted">via {node.indexName}</span> : null}
      </div>
      <div className="muted code" style={{ marginLeft: 4 }}>
        rows: plan {node.rows} / actual {node.actualRows} · cost {node.totalCost.toFixed(1)} · time {node.actualTimeMs.toFixed(2)} ms
      </div>
      {node.indexCond ? (
        <div className="code" style={{ marginLeft: 4 }}>cond: {node.indexCond}</div>
      ) : null}
      {node.filter ? (
        <div className="code" style={{ marginLeft: 4 }}>filter: {node.filter}</div>
      ) : null}
      {node.children?.map((c, i) => (
        <PlanNode key={i} node={c} depth={depth + 1} />
      ))}
    </div>
  );
}

export function PlanView({ title, report }: Props) {
  if (!report) {
    return (
      <div className="panel" style={{ flex: 1, minWidth: 320 }}>
        <div className="panel-title">{title}</div>
        <div className="muted">No plan yet.</div>
      </div>
    );
  }
  return (
    <div className="panel" style={{ flex: 1, minWidth: 320 }}>
      <div className="panel-title row" style={{ justifyContent: 'space-between' }}>
        <span>{title}</span>
        <span className="muted" style={{ fontSize: 11, textTransform: 'none' }}>
          {report.executionTimeMs.toFixed(2)} ms · planning {report.planningTimeMs.toFixed(2)} ms
        </span>
      </div>
      <PlanNode node={report.tree} />
      <div style={{ marginTop: 10 }}>
        <div className="panel-title">Notes</div>
        <ul style={{ margin: 0, paddingLeft: 18 }}>
          {report.highlights.map((h, i) => (
            <li key={i} style={{ fontSize: 12, marginBottom: 4 }}>{h}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}
