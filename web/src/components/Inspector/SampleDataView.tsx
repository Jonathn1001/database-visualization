// Lazy-loaded sample-data table (§14.3). Fetches GET /tables/:nodeId/sample via
// react-query and renders masked/unmasked rows. PII masking is the default
// (§22.3); a "Show full data" toggle opens a confirmation dialog warning about
// PII, and the unmasked state is per-session — it resets when nodeId/connId
// changes (DESIGN_PLAN §22.3).
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { AnimatePresence, motion } from 'framer-motion';
import { Eye, EyeOff, ShieldAlert } from 'lucide-react';

import * as schemaApi from '@/api/schema';
import { SAMPLE_DEFAULT_LIMIT } from '@/lib/constants';

export function SampleDataView({
  connId,
  nodeId,
}: {
  connId: string;
  nodeId: string;
}) {
  // Per-session unmask flag. Resets whenever the selected table or the
  // connection changes (§22.3).
  const [showFull, setShowFull] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    setShowFull(false);
    setConfirmOpen(false);
  }, [connId, nodeId]);

  const query = useQuery({
    queryKey: ['sample', connId, nodeId, showFull],
    queryFn: () =>
      schemaApi.sample(connId, nodeId, SAMPLE_DEFAULT_LIMIT, showFull),
  });

  const data = query.data;
  // rows can be null when the backend returns an empty result for a 0-row table;
  // guard so an empty table renders "No rows" instead of crashing on .length.
  const rows = data?.rows ?? [];
  const columns = rows.length > 0 ? Object.keys(rows[0]) : [];

  return (
    <section className="border-t border-black/10 px-3 py-3 dark:border-white/10">
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          Sample data
        </h3>
        {data?.masked && !showFull ? (
          <button
            type="button"
            onClick={() => setConfirmOpen(true)}
            className="flex items-center gap-1 rounded border border-black/10 px-2 py-0.5 text-xs text-neutral-600 hover:bg-black/5 dark:border-white/10 dark:text-neutral-300 dark:hover:bg-white/5"
          >
            <Eye size={13} />
            Show full data
          </button>
        ) : showFull ? (
          <button
            type="button"
            onClick={() => setShowFull(false)}
            className="flex items-center gap-1 rounded border border-black/10 px-2 py-0.5 text-xs text-neutral-600 hover:bg-black/5 dark:border-white/10 dark:text-neutral-300 dark:hover:bg-white/5"
          >
            <EyeOff size={13} />
            Hide
          </button>
        ) : null}
      </div>

      {query.isLoading && (
        <div className="py-4 text-xs text-neutral-500">Loading sample…</div>
      )}

      {query.isError && (
        <div className="py-4 text-xs text-red-600">
          Failed to load sample data.
        </div>
      )}

      {data && rows.length === 0 && (
        <div className="py-4 text-xs text-neutral-500">No rows.</div>
      )}

      {data && rows.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-black/10 dark:border-white/10">
                {columns.map((col) => (
                  <th
                    key={col}
                    className="whitespace-nowrap px-2 py-1 font-medium text-neutral-500"
                  >
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                <tr
                  key={i}
                  className="border-b border-black/5 last:border-0 dark:border-white/5"
                >
                  {columns.map((col) => (
                    <td
                      key={col}
                      className="max-w-[12rem] truncate px-2 py-1 font-mono text-neutral-700 dark:text-neutral-300"
                      title={formatCell(row[col])}
                    >
                      {formatCell(row[col])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data?.masked && !showFull && (
        <p className="mt-2 flex items-center gap-1 text-[11px] text-neutral-400">
          <ShieldAlert size={12} />
          Potential PII is masked.
        </p>
      )}

      <ConfirmDialog
        open={confirmOpen}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => {
          setShowFull(true);
          setConfirmOpen(false);
        }}
      />
    </section>
  );
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return '∅';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function ConfirmDialog({
  open,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onCancel}
        >
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-labelledby="pii-confirm-title"
            className="w-full max-w-sm rounded-lg border border-black/10 bg-canvas-light p-4 text-sm text-neutral-900 shadow-lg dark:border-white/10 dark:bg-canvas-dark dark:text-neutral-100"
            initial={{ opacity: 0, scale: 0.96, y: 8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 8 }}
            transition={{ duration: 0.15 }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-2 flex items-center gap-2">
              <ShieldAlert size={18} className="text-amber-600" />
              <h2 id="pii-confirm-title" className="text-sm font-semibold">
                Reveal unmasked data?
              </h2>
            </div>
            <p className="mb-4 text-sm text-neutral-600 dark:text-neutral-300">
              This may expose personally identifiable information (emails,
              phone numbers, and other sensitive values). Only reveal it if you
              are authorized to view this data. This stays unmasked for this
              session only.
            </p>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={onCancel}
                className="rounded border border-black/10 px-3 py-1 text-xs hover:bg-black/5 dark:border-white/10 dark:hover:bg-white/5"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={onConfirm}
                className="rounded bg-amber-600 px-3 py-1 text-xs font-medium text-white hover:bg-amber-700"
              >
                Show full data
              </button>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
