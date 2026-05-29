// Typed wrappers for the simulation endpoints (§6.3).
import { api, encodeNodeId } from './client';
import type { CascadeResult, CrudFlowResult, CrudOp, QueryPlan } from '@/types/graph';

// GET /api/connections/:id/simulate/cascade/:nodeId -> CascadeResult
export function cascade(connId: string, nodeId: string): Promise<CascadeResult> {
  return api<CascadeResult>(
    `/api/connections/${encodeURIComponent(connId)}/simulate/cascade/${encodeNodeId(nodeId)}`,
  );
}

// POST /api/connections/:id/simulate/crud { operation, nodeId } -> CrudFlowResult
export function crud(
  connId: string,
  nodeId: string,
  operation: CrudOp,
): Promise<CrudFlowResult> {
  return api<CrudFlowResult>(
    `/api/connections/${encodeURIComponent(connId)}/simulate/crud`,
    {
      method: 'POST',
      body: JSON.stringify({ operation, nodeId }),
    },
  );
}

// POST /api/connections/:id/explain { query } -> QueryPlan
export function explain(connId: string, query: string): Promise<QueryPlan> {
  return api<QueryPlan>(`/api/connections/${encodeURIComponent(connId)}/explain`, {
    method: 'POST',
    body: JSON.stringify({ query }),
  });
}
