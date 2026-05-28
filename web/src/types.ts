export interface TraceEvent {
  type: string;
  label?: string;
  notes?: string;
  eventPhase?: string;
  operationId?: string;
  details?: Record<string, unknown>;
}

export interface NodeView {
  pageId: number;
  isLeaf: boolean;
  keys: number[];
  values?: string[];
  childPageIds?: number[];
  nextPageId?: number;
  level: number;
}

export interface Snapshot {
  order: number;
  size: number;
  height: number;
  maxKeys: number;
  minKeys: number;
  rootPageId: number;
  nodes: NodeView[];
  levels: NodeView[][];
  leafChain: number[];
  diskReads: number;
  diskWrites: number;
}

export interface KV {
  key: number;
  value: string;
}

export interface OpResponse {
  operation: string;
  key?: number;
  lo?: number;
  hi?: number;
  value?: string;
  found?: boolean;
  results?: KV[];
  metrics?: BPTreeMetrics;
  pre?: BPTreeSnapshotSummary;
  after?: BPTreeSnapshotSummary;
  scenarioRun?: ScenarioRun;
  eventGroups?: ScenarioStep[];
  trace: TraceEvent[];
  snapshot: Snapshot;
}

export interface BPTreeSnapshotSummary {
  order: number;
  size: number;
  height: number;
  rootPageId: number;
  diskReads: number;
  diskWrites: number;
}

export interface BPTreeMetrics {
  pathLength: number;
  nodeReads: number;
  nodeWrites: number;
  eventCountsByType: Record<string, number>;
  invariantChecksPassed: boolean;
}

export interface ScenarioStep {
  step: number;
  op: string;
  label?: string;
  notes?: string;
  key?: number;
  lo?: number;
  hi?: number;
  metrics: BPTreeMetrics;
  eventFrom: number;
  eventTo: number;
  resultFound?: boolean;
  resultCount?: number;
  pre?: BPTreeSnapshotSummary;
  after?: BPTreeSnapshotSummary;
}

export interface ScenarioRun {
  name?: string;
  totalSteps: number;
  timestampMs: number;
  steps: ScenarioStep[];
}

export interface IndexInfo {
  name: string;
  table: string;
  definition: string;
  sizeBytes: number;
}

export interface ScanInfo {
  nodeType: string;
  relation?: string;
  indexName?: string;
  indexCond?: string;
  filter?: string;
  rows: number;
  actualRows: number;
  startupCost?: number;
  totalCost: number;
  actualTimeMs: number;
  loops?: number;
}

export interface PlanNodeT {
  nodeType: string;
  relation?: string;
  indexName?: string;
  indexCond?: string;
  filter?: string;
  rows: number;
  actualRows: number;
  totalCost: number;
  actualTimeMs: number;
  children?: PlanNodeT[];
}

export interface PlanReport {
  planningTimeMs: number;
  executionTimeMs: number;
  totalCost: number;
  rows: number;
  scans: ScanInfo[];
  highlights: string[];
  tree: PlanNodeT;
}

export interface Recommendation {
  table: string;
  columns: string[];
  reason: string;
  sql: string;
}
