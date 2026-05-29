// Typed wrappers for connection-management endpoints (§6.1, §6.4).
import { api } from './client';
import type { ConnectionConfig, ConnMeta, DockerContainer } from '@/types/graph';

// POST /api/connections -> ConnMeta (201)
export function create(config: ConnectionConfig): Promise<ConnMeta> {
  return api<ConnMeta>('/api/connections', {
    method: 'POST',
    body: JSON.stringify(config),
  });
}

// GET /api/connections -> ConnMeta[]
export function list(): Promise<ConnMeta[]> {
  return api<ConnMeta[]>('/api/connections');
}

// GET /api/connections/:id -> ConnMeta
export function get(id: string): Promise<ConnMeta> {
  return api<ConnMeta>(`/api/connections/${encodeURIComponent(id)}`);
}

// DELETE /api/connections/:id -> 204
export function del(id: string): Promise<void> {
  return api<void>(`/api/connections/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

// POST /api/connections/test -> { ok, engine }
export function test(config: ConnectionConfig): Promise<{ ok: true; engine: string }> {
  return api<{ ok: true; engine: string }>('/api/connections/test', {
    method: 'POST',
    body: JSON.stringify(config),
  });
}

// POST /api/connections/:id/ping -> { status: "ok" }
export function ping(id: string): Promise<{ status: string }> {
  return api<{ status: string }>(`/api/connections/${encodeURIComponent(id)}/ping`, {
    method: 'POST',
  });
}

// GET /api/docker/containers -> { containers }
export function listDockerContainers(): Promise<DockerContainer[]> {
  return api<{ containers: DockerContainer[] }>('/api/docker/containers').then(
    (r) => r.containers,
  );
}
