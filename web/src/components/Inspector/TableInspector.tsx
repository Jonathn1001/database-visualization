// Right-hand inspector panel. Reads the selected nodeId from the selection
// store; when a node is selected it renders the table's metadata + columns and
// lazily mounts the sample-data view below (DESIGN_PLAN §6.2, §22.3). When no
// node is selected it shows a hint. Do NOT change the exported signature.
import { lazy, Suspense } from 'react';
import { KeyRound, Link2, Hash } from 'lucide-react';

import type { Column, GraphModel } from '@/types/graph';
import { useSelectionStore } from '@/store/selection';

// SampleDataView is a named export, so adapt it for React.lazy (§14.3).
const SampleDataView = lazy(() =>
  import('./SampleDataView').then((m) => ({ default: m.SampleDataView })),
);

export function TableInspector({
  connId,
  graph,
}: {
  connId: string;
  graph: GraphModel;
}) {
  const selectedNodeId = useSelectionStore((s) => s.selectedNodeId);
  const node =
    selectedNodeId !== null
      ? graph.nodes.find((n) => n.id === selectedNodeId)
      : undefined;

  if (!node) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6 text-center text-sm text-neutral-500">
        Select a table in the graph to inspect its columns and sample data.
      </div>
    );
  }

  return (
    <div className="flex h-full w-full flex-col">
      {/* Header: label + kind + row count + size */}
      <header className="shrink-0 border-b border-black/10 px-3 py-3 dark:border-white/10">
        <div className="flex items-baseline gap-2">
          <h2 className="truncate text-sm font-semibold" title={node.id}>
            {node.label}
          </h2>
          <span className="rounded bg-black/5 px-1.5 py-0.5 text-[11px] uppercase tracking-wide text-neutral-500 dark:bg-white/10">
            {node.kind}
          </span>
        </div>
        <dl className="mt-1.5 flex gap-4 text-xs text-neutral-500">
          <div className="flex items-baseline gap-1">
            <dt>Rows</dt>
            <dd className="font-medium text-neutral-700 dark:text-neutral-300">
              {formatCount(node.rowCount)}
            </dd>
          </div>
          <div className="flex items-baseline gap-1">
            <dt>Size</dt>
            <dd className="font-medium text-neutral-700 dark:text-neutral-300">
              {formatBytes(node.sizeBytes)}
            </dd>
          </div>
        </dl>
      </header>

      {/* Columns */}
      <div className="px-3 py-3">
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">
          Columns ({node.columns.length})
        </h3>
        <ul className="space-y-2">
          {node.columns.map((col) => (
            <ColumnRow key={col.name} col={col} />
          ))}
        </ul>
      </div>

      {/* Sample data (lazy) */}
      <Suspense
        fallback={
          <div className="border-t border-black/10 px-3 py-3 text-xs text-neutral-500 dark:border-white/10">
            Loading sample…
          </div>
        }
      >
        <SampleDataView connId={connId} nodeId={node.id} />
      </Suspense>
    </div>
  );
}

function ColumnRow({ col }: { col: Column }) {
  return (
    <li className="text-xs">
      <div className="flex items-center gap-1.5">
        <span className="truncate font-medium text-neutral-800 dark:text-neutral-200">
          {col.name}
        </span>
        <span className="font-mono text-neutral-500">{col.type}</span>
        {col.nullable ? (
          <span className="text-[11px] text-neutral-400">null</span>
        ) : (
          <span className="text-[11px] text-neutral-400">not null</span>
        )}
        <span className="ml-auto flex items-center gap-1">
          {col.isPk && (
            <Badge title="Primary key" tone="amber">
              <KeyRound size={10} />
              PK
            </Badge>
          )}
          {col.isFk && (
            <Badge title="Foreign key" tone="blue">
              <Link2 size={10} />
              FK
            </Badge>
          )}
          {col.indexes && col.indexes.length > 0 && (
            <Badge
              title={`Indexed: ${col.indexes.join(', ')}`}
              tone="neutral"
            >
              <Hash size={10} />
              IDX
            </Badge>
          )}
        </span>
      </div>
      {col.isFk && col.fkRef && (
        <div className="mt-0.5 pl-0.5 text-[11px] text-neutral-500">
          →{' '}
          <span className="font-mono text-neutral-600 dark:text-neutral-400">
            {col.fkRef.table}.{col.fkRef.column}
          </span>
          {col.fkRef.onDelete && (
            <span className="ml-1 text-neutral-400">
              ON DELETE {col.fkRef.onDelete}
            </span>
          )}
          {col.fkRef.inferred && (
            <span className="ml-1 italic text-neutral-400">(inferred)</span>
          )}
        </div>
      )}
    </li>
  );
}

function Badge({
  children,
  title,
  tone,
}: {
  children: React.ReactNode;
  title: string;
  tone: 'amber' | 'blue' | 'neutral';
}) {
  const toneClass =
    tone === 'amber'
      ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200'
      : tone === 'blue'
        ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-200'
        : 'bg-black/5 text-neutral-600 dark:bg-white/10 dark:text-neutral-300';
  return (
    <span
      title={title}
      className={`flex items-center gap-0.5 rounded px-1 py-0.5 text-[10px] font-medium ${toneClass}`}
    >
      {children}
    </span>
  );
}

function formatCount(n: number): string {
  return n.toLocaleString();
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
