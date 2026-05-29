// Canonical frontend types. These mirror the Go structs in
// internal/model/{graph,plan,connection,errors}.go and internal/simulate/cascade.go.
// Field names MUST stay in sync with the JSON tags on those structs — do not rename.
// This is the frozen shared contract for all feature agents (DESIGN_PLAN §2.3, §8).

export type Engine = 'postgres' | 'mongodb' | 'sqlite';

export interface ConnectionConfig {
  engine: Engine | '';
  dsn?: string;
  host?: string;
  port?: number;
  user?: string;
  password?: string;
  database?: string;
  filePath?: string;
  label?: string;
}

export interface ConnMeta {
  id: string;
  engine: string;
  state: string;
  config: ConnectionConfig;
  openedAt: string;
}

export interface FKRef {
  table: string;
  column: string;
  onDelete: string;
  onUpdate: string;
  inferred: boolean;
  confidence?: number;
}

export interface Column {
  name: string;
  type: string;
  nullable: boolean;
  isPk: boolean;
  isFk: boolean;
  fkRef?: FKRef;
  indexes?: string[];
  default?: string;
}

export interface Node {
  id: string;
  schema: string;
  label: string;
  kind: string;
  columns: Column[];
  rowCount: number;
  sizeBytes: number;
  tenant?: string;
}

export interface Link {
  source: string;
  target: string;
  type: string;
  cascade: boolean;
  viaColumn: string;
  inferred: boolean;
  confidence?: number;
}

export interface Stats {
  nodeCount: number;
  linkCount: number;
  cascadeChainDepth: number;
  columnCount: number;
}

export interface Warning {
  code: string;
  message: string;
}

export interface GraphModel {
  engine: string;
  database: string;
  schemas: string[];
  nodes: Node[];
  links: Link[];
  stats: Stats;
  warnings?: Warning[];
}

export interface TableStats {
  nodeId: string;
  rowCount: number;
  deadTuples?: number;
  sizeBytes: number;
  inserts?: number;
  updates?: number;
  deletes?: number;
  lastVacuum?: string;
  lastAutovacuum?: string;
  lastAnalyze?: string;
}

export interface CascadeStep {
  nodeId: string;
  via: string;
  viaColumn: string;
  depth: number;
}

export interface CascadeResult {
  root: string;
  affected: CascadeStep[];
}

export interface ExplainNode {
  table: string;
  op: string;
  cost: number;
}

export interface QueryPlan {
  query: string;
  nodes: ExplainNode[];
}

export type CrudOp = 'insert' | 'update' | 'delete';

export interface CrudFlowStep {
  nodeId: string;
  order: number;
  action: 'insert' | 'update' | 'delete' | 'check';
  via?: string;
  viaColumn?: string;
}

export interface CrudFlowResult {
  operation: CrudOp;
  root: string;
  steps: CrudFlowStep[];
}

export interface DockerContainer {
  id: string;
  name: string;
  image: string;
  engine: string;
  ports: string[];
}

export type ErrorCode =
  | 'CONN_INVALID_DSN'
  | 'CONN_AUTH_FAILED'
  | 'CONN_UNREACHABLE'
  | 'CONN_SUPERUSER_REJECTED'
  | 'CONN_NOT_FOUND'
  | 'CONN_TIMEOUT'
  | 'CONN_ALREADY_CLOSED'
  | 'SCHEMA_INTROSPECT_FAILED'
  | 'SCHEMA_PERMISSION_DENIED'
  | 'SCHEMA_NOT_FOUND'
  | 'SCHEMA_TOO_LARGE'
  | 'EXPLAIN_INVALID_SQL'
  | 'EXPLAIN_NOT_SUPPORTED'
  | 'EXPLAIN_TIMEOUT'
  | 'ADAPTER_NOT_SUPPORTED'
  | 'ADAPTER_OPERATION_NOT_SUPPORTED'
  | 'ADAPTER_READONLY_VIOLATION'
  | 'INTERNAL'
  | 'BAD_REQUEST'
  | 'RATE_LIMITED';

// Imperative handle the graph registers so the ActionBar can drive animations without prop-drilling the D3 sim.
export interface GraphAnimator {
  animateCascade: (r: CascadeResult) => void;
  animateQueryPath: (p: QueryPlan) => void;
  animateCrud: (r: CrudFlowResult) => void;
  pulse: (nodeId: string) => void;
  clearHighlights: () => void;
}
