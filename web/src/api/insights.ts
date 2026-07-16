// Typed wrapper for the insights endpoint (pg-insights spec, phase 1).
import { api } from './client';
import type { InsightsResult } from '@/types/graph';

// GET /api/connections/:id/insights(?category=index|health|gaps) -> InsightsResult
export function getInsights(connId: string, category?: string): Promise<InsightsResult> {
  const qs = category ? `?category=${encodeURIComponent(category)}` : '';
  return api<InsightsResult>(`/api/connections/${encodeURIComponent(connId)}/insights${qs}`);
}
