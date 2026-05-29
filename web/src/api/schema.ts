// Typed wrappers for schema introspection + per-table endpoints (§6.2).
import { api, encodeNodeId } from './client';
import type { GraphModel, TableStats } from '@/types/graph';

// GET /api/connections/:id/schema?schemas=public,audit -> GraphModel
export function getSchema(connId: string, schemas?: string[]): Promise<GraphModel> {
  const qs = schemas && schemas.length > 0 ? `?schemas=${encodeURIComponent(schemas.join(','))}` : '';
  return api<GraphModel>(`/api/connections/${encodeURIComponent(connId)}/schema${qs}`);
}

// GET /api/connections/:id/schema/refresh -> GraphModel
export function refreshSchema(connId: string): Promise<GraphModel> {
  return api<GraphModel>(`/api/connections/${encodeURIComponent(connId)}/schema/refresh`);
}

// GET /api/connections/:id/schemas -> { schemas }
export function listSchemas(connId: string): Promise<string[]> {
  return api<{ schemas: string[] }>(
    `/api/connections/${encodeURIComponent(connId)}/schemas`,
  ).then((r) => r.schemas);
}

// GET /api/connections/:id/tables/:nodeId/sample?limit=5(&full=true) -> { rows, masked }
export function sample(
  connId: string,
  nodeId: string,
  limit = 5,
  full = false,
): Promise<{ rows: Record<string, unknown>[]; masked: boolean }> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (full) params.set('full', 'true');
  return api<{ rows: Record<string, unknown>[]; masked: boolean }>(
    `/api/connections/${encodeURIComponent(connId)}/tables/${encodeNodeId(nodeId)}/sample?${params.toString()}`,
  );
}

// GET /api/connections/:id/tables/:nodeId/stats -> TableStats
export function stats(connId: string, nodeId: string): Promise<TableStats> {
  return api<TableStats>(
    `/api/connections/${encodeURIComponent(connId)}/tables/${encodeNodeId(nodeId)}/stats`,
  );
}
