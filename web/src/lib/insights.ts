// Pure helpers shared by the InsightsPanel and the graph overlay.
import type { Finding, GraphModel, InsightSeverity, InsightsResult, Link } from '@/types/graph';

const SEVERITY_RANK: Record<string, number> = { critical: 0, warn: 1, info: 2 };

/** Lower rank = more severe. Unknown severities sort last. */
export function severityRank(severity: string): number {
  return SEVERITY_RANK[severity] ?? 3;
}

/** Returns a new array sorted by severity-then-table or table-then-severity. */
export function sortFindings(findings: Finding[], by: 'severity' | 'table'): Finding[] {
  return [...findings].sort((a, b) => {
    if (by === 'table') {
      return a.nodeId.localeCompare(b.nodeId) || severityRank(a.severity) - severityRank(b.severity);
    }
    return severityRank(a.severity) - severityRank(b.severity) || a.nodeId.localeCompare(b.nodeId);
  });
}

/** Extracts the gap candidates' inferred edges from a result. */
export function impliedLinks(result: InsightsResult | null): Link[] {
  return result?.categories.find((c) => c.category === 'gaps')?.impliedLinks ?? [];
}

const linkKey = (l: Link) => `${l.source}->${l.target}:${l.viaColumn}`;

/**
 * mergeImpliedLinks returns a copy of the graph with implied edges appended.
 * Links whose endpoints are missing from the graph, or that duplicate a real
 * link, are dropped. Returns the input graph unchanged when nothing merges.
 */
export function mergeImpliedLinks(graph: GraphModel, links: Link[]): GraphModel {
  if (links.length === 0) return graph;
  const nodeIds = new Set(graph.nodes.map((n) => n.id));
  const real = new Set(graph.links.map(linkKey));
  const extra = links.filter(
    (l) => nodeIds.has(l.source) && nodeIds.has(l.target) && !real.has(linkKey(l)),
  );
  if (extra.length === 0) return graph;
  return { ...graph, links: [...graph.links, ...extra] };
}

/** Highest severity per node across all supported categories. */
export function severityByNode(result: InsightsResult | null): Map<string, InsightSeverity> {
  const map = new Map<string, InsightSeverity>();
  if (!result) return map;
  for (const cat of result.categories) {
    if (cat.status !== 'ok') continue;
    for (const f of cat.findings) {
      const current = map.get(f.nodeId);
      if (!current || severityRank(f.severity) < severityRank(current)) {
        map.set(f.nodeId, f.severity);
      }
    }
  }
  return map;
}
