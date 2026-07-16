// App shell: wires providers, the desktop gate, and the connect -> workspace
// flow against the frozen component stubs (DESIGN_PLAN §13). Feature agents fill
// in the component bodies; this shell composition stays stable.
import { useEffect, useMemo, useState } from 'react';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import { X } from 'lucide-react';

import DesktopOnlyGate from '@/components/DesktopOnlyGate';
import { ConnectionWizard } from '@/components/ConnectionWizard';
import { GraphCanvas } from '@/components/Graph/GraphCanvas';
import { TableInspector } from '@/components/Inspector/TableInspector';
import { InsightsPanel } from '@/components/InsightsPanel';
import { ActionBar } from '@/components/ActionBar';
import { SchemaPicker } from '@/components/SchemaPicker';
import { ConnectionStatus } from '@/components/ConnectionStatus';

import { useConnectionStore } from '@/store/connection';
import { useInsightsStore } from '@/store/insights';
import { impliedLinks, mergeImpliedLinks } from '@/lib/insights';
import * as schemaApi from '@/api/schema';
import type { ConnMeta, GraphModel, Warning } from '@/types/graph';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <DesktopOnlyGate>
        <Shell />
      </DesktopOnlyGate>
      <Toaster position="bottom-right" richColors closeButton />
    </QueryClientProvider>
  );
}

function Shell() {
  const conn = useConnectionStore((s) => s.conn);
  const setConn = useConnectionStore((s) => s.setConn);
  const setSchemas = useConnectionStore((s) => s.setSchemas);

  if (conn === null) {
    return (
      <ConnectionWizard
        onConnected={(c: ConnMeta) => {
          setConn(c);
          setSchemas(c.config.database ? [] : []);
        }}
      />
    );
  }

  return <Workspace conn={conn} />;
}

function Workspace({ conn }: { conn: ConnMeta }) {
  const schemas = useConnectionStore((s) => s.schemas);
  const setSchemas = useConnectionStore((s) => s.setSchemas);
  const clearConn = useConnectionStore((s) => s.clearConn);
  const [dismissedWarnings, setDismissedWarnings] = useState<Set<string>>(new Set());
  const [sideTab, setSideTab] = useState<'inspector' | 'insights'>('inspector');

  // Insights are per-connection: drop stale results + overlay when it changes.
  useEffect(() => {
    useInsightsStore.getState().clear();
    setSideTab('inspector');
  }, [conn.id]);

  // Available schemas for the picker.
  const schemasQuery = useQuery({
    queryKey: ['schemas', conn.id],
    queryFn: () => schemaApi.listSchemas(conn.id),
  });

  // The graph model for the currently-loaded schemas.
  const graphQuery = useQuery({
    queryKey: ['schema', conn.id, schemas],
    queryFn: () => schemaApi.getSchema(conn.id, schemas.length > 0 ? schemas : undefined),
  });

  const insightsResult = useInsightsStore((s) => s.result);
  const overlay = useInsightsStore((s) => s.overlay);

  // With the overlay on, gap candidates are appended as inferred links — the
  // existing dashed renderer picks them up via link.inferred (§2.3 contract).
  const displayGraph = useMemo(() => {
    if (!graphQuery.data) return undefined;
    if (!overlay) return graphQuery.data;
    return mergeImpliedLinks(graphQuery.data, impliedLinks(insightsResult));
  }, [graphQuery.data, overlay, insightsResult]);

  return (
    <div className="flex h-screen flex-col bg-canvas-light text-neutral-900 dark:bg-canvas-dark dark:text-neutral-100">
      {/* Top bar */}
      <header className="flex shrink-0 items-center gap-4 border-b border-black/10 px-4 py-2 dark:border-white/10">
        <span className="text-sm font-semibold">
          {conn.config.label || conn.config.database || conn.engine}
        </span>
        <SchemaPicker
          connId={conn.id}
          current={schemas}
          available={schemasQuery.data ?? []}
          onChange={setSchemas}
        />
        <div className="ml-auto flex items-center gap-3">
          <ConnectionStatus connId={conn.id} />
          <button
            type="button"
            onClick={clearConn}
            className="rounded border border-black/10 px-2 py-1 text-xs hover:bg-black/5 dark:border-white/10 dark:hover:bg-white/5"
          >
            Disconnect
          </button>
        </div>
      </header>

      {/* Warnings banner */}
      <WarningBanner
        warnings={graphQuery.data?.warnings ?? []}
        dismissed={dismissedWarnings}
        onDismiss={(code) =>
          setDismissedWarnings((prev) => {
            const next = new Set(prev);
            next.add(code);
            return next;
          })
        }
      />

      {/* Main area: graph + right inspector */}
      <div className="flex min-h-0 flex-1">
        <main className="relative min-w-0 flex-1">
          {graphQuery.isLoading && (
            <div className="flex h-full items-center justify-center text-sm text-neutral-500">
              Loading schema…
            </div>
          )}
          {graphQuery.isError && (
            <div className="flex h-full items-center justify-center text-sm text-red-600">
              Failed to load schema.
            </div>
          )}
          {displayGraph && <GraphCanvas graph={displayGraph} />}
        </main>

        <aside className="flex w-80 shrink-0 flex-col overflow-hidden border-l border-black/10 dark:border-white/10">
          <div className="flex shrink-0 gap-1 border-b border-black/10 px-2 py-1.5 dark:border-white/10">
            <SideTabButton
              active={sideTab === 'inspector'}
              onClick={() => setSideTab('inspector')}
              label="Inspector"
            />
            <SideTabButton
              active={sideTab === 'insights'}
              onClick={() => setSideTab('insights')}
              label="Insights"
            />
          </div>
          <div className="min-h-0 flex-1 overflow-auto">
            {graphQuery.data &&
              (sideTab === 'inspector' ? (
                <TableInspector connId={conn.id} graph={graphQuery.data} />
              ) : (
                <InsightsPanel connId={conn.id} graph={graphQuery.data} />
              ))}
          </div>
        </aside>
      </div>

      {/* Bottom action bar */}
      <footer className="h-40 shrink-0 border-t border-black/10 dark:border-white/10">
        {graphQuery.data && <ActionBar connId={conn.id} graph={graphQuery.data} />}
      </footer>
    </div>
  );
}

function SideTabButton({
  active,
  onClick,
  label,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        'rounded px-2.5 py-0.5 text-xs font-medium transition-colors ' +
        (active
          ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
          : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
      }
    >
      {label}
    </button>
  );
}

function WarningBanner({
  warnings,
  dismissed,
  onDismiss,
}: {
  warnings: Warning[];
  dismissed: Set<string>;
  onDismiss: (code: string) => void;
}) {
  const visible = warnings.filter((w) => !dismissed.has(w.code));
  if (visible.length === 0) return null;
  return (
    <div className="shrink-0">
      {visible.map((w) => (
        <div
          key={w.code}
          className="flex items-center gap-2 border-b border-amber-300/50 bg-amber-100/60 px-4 py-1.5 text-sm text-amber-900 dark:bg-amber-900/30 dark:text-amber-200"
        >
          <span>{w.message}</span>
          <button
            type="button"
            aria-label="Dismiss warning"
            onClick={() => onDismiss(w.code)}
            className="ml-auto rounded p-0.5 hover:bg-black/10 dark:hover:bg-white/10"
          >
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}

// Re-export for convenience / type checks of the GraphModel-driven workspace.
export type { GraphModel };
