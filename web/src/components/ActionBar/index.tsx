// Bottom action bar (DESIGN_PLAN §13.2, §6.3). Drives graph animations by
// reading the imperative GraphAnimator handle the GraphCanvas registers in the
// graph store, plus the currently-selected node from the selection store.
// Three groups of controls:
//   - Cascade delete  -> GET  /simulate/cascade/:nodeId -> animator.animateCascade
//   - CRUD flow       -> POST /simulate/crud            -> animator.animateCrud
//   - Query Path tab  -> POST /explain                  -> animator.animateQueryPath
// The tool is read-only: these only *simulate* and animate; nothing is written.
import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  Trash2,
  Plus,
  Pencil,
  GitBranch,
  Route,
  MousePointerClick,
  Lightbulb,
} from 'lucide-react';

import * as simulate from '@/api/simulate';
import { APIError } from '@/api/client';
import { useGraphStore } from '@/store/graph';
import { useSelectionStore } from '@/store/selection';
import { useInsightsStore } from '@/store/insights';
import { QueryPathInput } from './QueryPathInput';
import type {
  GraphModel,
  CrudOp,
  CascadeResult,
  CrudFlowResult,
  QueryPlan,
} from '@/types/graph';

type Tab = 'simulate' | 'query';

export function ActionBar({ connId, graph }: { connId: string; graph: GraphModel }) {
  const [tab, setTab] = useState<Tab>('simulate');

  const animator = useGraphStore((s) => s.animator);
  const selectedNodeId = useSelectionStore((s) => s.selectedNodeId);
  const overlay = useInsightsStore((s) => s.overlay);
  const setOverlay = useInsightsStore((s) => s.setOverlay);
  const hasInsights = useInsightsStore((s) => s.result !== null);

  const selectedNode = selectedNodeId
    ? graph.nodes.find((n) => n.id === selectedNodeId)
    : undefined;
  const selectedLabel = selectedNode?.label ?? selectedNodeId ?? null;

  // --- Cascade delete simulation -------------------------------------------
  const cascadeMutation = useMutation<CascadeResult, unknown, string>({
    mutationFn: (nodeId) => simulate.cascade(connId, nodeId),
    onSuccess: (result) => {
      animator?.animateCascade(result);
      const count = result.affected.length;
      if (count === 0) {
        toast.info('No cascading deletes — nothing else is affected.');
      } else {
        toast.success(
          `Cascade affects ${count} ${count === 1 ? 'table' : 'tables'}.`,
        );
      }
    },
    onError: showError,
  });

  // --- CRUD flow simulation -------------------------------------------------
  const crudMutation = useMutation<
    CrudFlowResult,
    unknown,
    { nodeId: string; op: CrudOp }
  >({
    mutationFn: ({ nodeId, op }) => simulate.crud(connId, nodeId, op),
    onSuccess: (result) => {
      animator?.animateCrud(result);
      const count = result.steps.length;
      toast.success(
        `${labelForOp(result.operation)} flow — ${count} ${
          count === 1 ? 'step' : 'steps'
        }.`,
      );
    },
    onError: showError,
  });

  // --- EXPLAIN / query-path simulation -------------------------------------
  const explainMutation = useMutation<QueryPlan, unknown, string>({
    mutationFn: (sql) => simulate.explain(connId, sql),
    onSuccess: (plan) => {
      animator?.animateQueryPath(plan);
      const count = plan.nodes.length;
      if (count === 0) {
        toast.info('Query plan touched no known tables.');
      } else {
        toast.success(
          `Query path — ${count} ${count === 1 ? 'table' : 'tables'}.`,
        );
      }
    },
    onError: (err) => {
      // EXPLAIN_INVALID_SQL gets a specific, friendly message (§8.3).
      if (err instanceof APIError && err.code === 'EXPLAIN_INVALID_SQL') {
        toast.error('Invalid SQL', {
          description: err.hint || err.message,
        });
        return;
      }
      showError(err);
    },
  });

  const hasSelection = selectedNodeId !== null;
  const busy =
    cascadeMutation.isPending ||
    crudMutation.isPending ||
    explainMutation.isPending;

  return (
    <div className="flex h-full w-full flex-col px-4 py-2 text-[13px]">
      {/* Tabs + selection context */}
      <div className="flex shrink-0 items-center gap-1">
        <TabButton
          active={tab === 'simulate'}
          onClick={() => setTab('simulate')}
          icon={<GitBranch size={13} />}
          label="Simulate"
        />
        <TabButton
          active={tab === 'query'}
          onClick={() => setTab('query')}
          icon={<Route size={13} />}
          label="Query Path"
        />
        <button
          type="button"
          disabled={!hasInsights}
          onClick={() => setOverlay(!overlay)}
          title={
            hasInsights
              ? 'Toggle insight badges + implied-FK edges on the graph'
              : 'Open the Insights panel first'
          }
          className={
            'inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40 ' +
            (overlay
              ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
              : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
          }
        >
          <Lightbulb size={13} />
          Insights
        </button>
        <div className="ml-auto flex items-center gap-1.5 text-[11px] text-neutral-500">
          <MousePointerClick size={12} />
          {selectedLabel ? (
            <span>
              Selected:{' '}
              <span className="font-medium text-neutral-700 dark:text-neutral-300">
                {selectedLabel}
              </span>
            </span>
          ) : (
            <span>Select a table to simulate cascade / CRUD</span>
          )}
        </div>
      </div>

      {/* Tab body */}
      <div className="mt-2 min-h-0 flex-1">
        {tab === 'simulate' ? (
          <div className="flex h-full items-center gap-6">
            {/* Cascade delete */}
            <div className="flex flex-col gap-1.5">
              <span className="text-[11px] font-medium uppercase tracking-wide text-neutral-500">
                Cascade delete
              </span>
              <button
                type="button"
                disabled={!hasSelection || busy}
                onClick={() =>
                  selectedNodeId && cascadeMutation.mutate(selectedNodeId)
                }
                className="inline-flex items-center gap-1.5 rounded border border-black/10 bg-white/70 px-3 py-1.5 font-medium hover:bg-black/5 disabled:cursor-not-allowed disabled:opacity-40 dark:border-white/10 dark:bg-white/10 dark:hover:bg-white/15"
                title="Simulate ON DELETE CASCADE from the selected table"
              >
                <Trash2 size={14} />
                {cascadeMutation.isPending ? 'Simulating…' : 'Simulate cascade'}
              </button>
            </div>

            <div className="h-10 w-px bg-black/10 dark:bg-white/10" />

            {/* CRUD flow */}
            <div className="flex flex-col gap-1.5">
              <span className="text-[11px] font-medium uppercase tracking-wide text-neutral-500">
                CRUD flow
              </span>
              <div className="flex items-center gap-1.5">
                <CrudButton
                  op="insert"
                  icon={<Plus size={14} />}
                  label="Insert"
                  disabled={!hasSelection || busy}
                  pending={
                    crudMutation.isPending &&
                    crudMutation.variables?.op === 'insert'
                  }
                  onClick={() =>
                    selectedNodeId &&
                    crudMutation.mutate({ nodeId: selectedNodeId, op: 'insert' })
                  }
                />
                <CrudButton
                  op="update"
                  icon={<Pencil size={14} />}
                  label="Update"
                  disabled={!hasSelection || busy}
                  pending={
                    crudMutation.isPending &&
                    crudMutation.variables?.op === 'update'
                  }
                  onClick={() =>
                    selectedNodeId &&
                    crudMutation.mutate({ nodeId: selectedNodeId, op: 'update' })
                  }
                />
                <CrudButton
                  op="delete"
                  icon={<Trash2 size={14} />}
                  label="Delete"
                  disabled={!hasSelection || busy}
                  pending={
                    crudMutation.isPending &&
                    crudMutation.variables?.op === 'delete'
                  }
                  onClick={() =>
                    selectedNodeId &&
                    crudMutation.mutate({ nodeId: selectedNodeId, op: 'delete' })
                  }
                />
              </div>
            </div>
          </div>
        ) : (
          <QueryPathInput
            onSubmit={(sql) => explainMutation.mutate(sql)}
            busy={explainMutation.isPending}
          />
        )}
      </div>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  icon,
  label,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        'inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium transition-colors ' +
        (active
          ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
          : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
      }
    >
      {icon}
      {label}
    </button>
  );
}

function CrudButton({
  icon,
  label,
  disabled,
  pending,
  onClick,
}: {
  op: CrudOp;
  icon: React.ReactNode;
  label: string;
  disabled?: boolean;
  pending?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="inline-flex items-center gap-1.5 rounded border border-black/10 bg-white/70 px-2.5 py-1.5 font-medium hover:bg-black/5 disabled:cursor-not-allowed disabled:opacity-40 dark:border-white/10 dark:bg-white/10 dark:hover:bg-white/15"
    >
      {icon}
      {pending ? '…' : label}
    </button>
  );
}

function labelForOp(op: CrudOp): string {
  switch (op) {
    case 'insert':
      return 'Insert';
    case 'update':
      return 'Update';
    case 'delete':
      return 'Delete';
  }
}

function showError(err: unknown) {
  if (err instanceof APIError) {
    toast.error(err.message, { description: err.hint });
    return;
  }
  toast.error(err instanceof Error ? err.message : 'Simulation failed.');
}
