// Insights side panel (pg-insights spec, phase 1). Mounting the panel (the
// user opening the tab) triggers the fetch — insights are on-demand only.
// The result is pushed into the insights store so the graph overlay
// (badges + implied edges) can read it.
import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ArrowUpDown, RefreshCw } from 'lucide-react';

import { getInsights } from '@/api/insights';
import { useGraphStore } from '@/store/graph';
import { useInsightsStore } from '@/store/insights';
import { useSelectionStore } from '@/store/selection';
import { sortFindings } from '@/lib/insights';
import { severityColor } from '@/lib/colors';
import type { Finding, GraphModel, InsightCategoryResult } from '@/types/graph';

const CATEGORY_LABELS: Record<string, string> = {
  index: 'Indexes',
  health: 'Health',
  gaps: 'Gaps',
};

export function InsightsPanel({ connId, graph }: { connId: string; graph: GraphModel }) {
  const [category, setCategory] = useState<string | null>(null);
  const [sortBy, setSortBy] = useState<'severity' | 'table'>('severity');
  const setResult = useInsightsStore((s) => s.setResult);

  const query = useQuery({
    queryKey: ['insights', connId],
    queryFn: () => getInsights(connId),
    staleTime: Infinity,
  });

  // Publish for the graph overlay.
  useEffect(() => {
    setResult(query.data ?? null);
  }, [query.data, setResult]);

  // Unsupported categories are hidden (spec: frontend hides that category).
  const visible = useMemo(
    () => (query.data?.categories ?? []).filter((c) => c.status === 'ok'),
    [query.data],
  );
  const active: InsightCategoryResult | undefined =
    visible.find((c) => c.category === category) ?? visible[0];

  const labelById = useMemo(
    () => new Map(graph.nodes.map((n) => [n.id, n.label])),
    [graph.nodes],
  );

  return (
    <div className="flex h-full flex-col text-[13px]">
      {/* Header: title + sort + refresh */}
      <div className="flex shrink-0 items-center gap-2 border-b border-black/10 px-3 py-2 dark:border-white/10">
        <span className="font-semibold">Insights</span>
        <button
          type="button"
          onClick={() => setSortBy(sortBy === 'severity' ? 'table' : 'severity')}
          title={`Sorted by ${sortBy} — click to toggle`}
          className="ml-auto inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5"
        >
          <ArrowUpDown size={11} />
          {sortBy}
        </button>
        <button
          type="button"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
          title="Re-run insights"
          className="rounded p-1 hover:bg-black/5 disabled:opacity-40 dark:hover:bg-white/5"
        >
          <RefreshCw size={13} className={query.isFetching ? 'animate-spin' : ''} />
        </button>
      </div>

      {/* Category tabs */}
      {visible.length > 0 && (
        <div className="flex shrink-0 gap-1 border-b border-black/10 px-2 py-1.5 dark:border-white/10">
          {visible.map((c) => (
            <button
              key={c.category}
              type="button"
              onClick={() => setCategory(c.category)}
              className={
                'rounded px-2 py-0.5 text-[11px] font-medium ' +
                (active?.category === c.category
                  ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
                  : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
              }
            >
              {CATEGORY_LABELS[c.category] ?? c.category} ({c.findings.length})
            </button>
          ))}
        </div>
      )}

      {/* Body */}
      <div className="min-h-0 flex-1 overflow-auto">
        {query.isLoading && (
          <p className="px-3 py-4 text-neutral-500">Analyzing schema…</p>
        )}
        {query.isError && (
          <p className="px-3 py-4 text-red-600">Failed to load insights.</p>
        )}
        {query.data && active && active.findings.length === 0 && (
          <p className="px-3 py-4 text-neutral-500">
            No {CATEGORY_LABELS[active.category]?.toLowerCase() ?? active.category} findings. 🎉
          </p>
        )}
        {active &&
          sortFindings(active.findings, sortBy).map((f, i) => (
            <FindingRow
              key={`${f.nodeId}-${f.title}-${i}`}
              finding={f}
              label={labelById.get(f.nodeId) ?? f.nodeId}
            />
          ))}
      </div>
    </div>
  );
}

function FindingRow({ finding, label }: { finding: Finding; label: string }) {
  const select = useSelectionStore((s) => s.select);
  const animator = useGraphStore((s) => s.animator);

  return (
    <button
      type="button"
      onClick={() => {
        select(finding.nodeId);
        animator?.pulse(finding.nodeId);
      }}
      title="Highlight this table on the graph"
      className="block w-full border-b border-black/5 px-3 py-2 text-left hover:bg-black/5 dark:border-white/5 dark:hover:bg-white/5"
    >
      <span className="flex items-center gap-1.5">
        <span
          aria-label={finding.severity}
          className="inline-block h-2 w-2 shrink-0 rounded-full"
          style={{ backgroundColor: severityColor(finding.severity) }}
        />
        <span className="font-medium">{finding.title}</span>
        <span className="ml-auto shrink-0 text-[11px] text-neutral-500">{label}</span>
      </span>
      <span className="mt-0.5 block text-[12px] text-neutral-600 dark:text-neutral-400">
        {finding.detail}
      </span>
    </button>
  );
}
