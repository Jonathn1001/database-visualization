// Connection wizard (DESIGN_PLAN §13.3, §8.3, §10.2). Three tabs — DSN (primary),
// Docker, and File — each gathers a ConnectionConfig and submits it via
// POST /api/connections. On success onConnected(meta) hands the new connection
// back to App.tsx, which swaps the wizard for the workspace.
//
// Error handling per §8.3:
//   CONN_SUPERUSER_REJECTED -> modal showing the create-role SQL (§10.2)
//   CONN_AUTH_FAILED / CONN_INVALID_DSN / CONN_UNREACHABLE -> inline error + toast
// All other codes surface as a toast.
import { useCallback, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { Database, Container, FileCode } from 'lucide-react';
import { toast } from 'sonner';

import * as connectionsApi from '@/api/connections';
import { APIError } from '@/api/client';
import type { ConnectionConfig, ConnMeta } from '@/types/graph';

import { DSNForm } from './DSNForm';
import { DockerPicker } from './DockerPicker';
import { FileForm } from './FileForm';
import { SuperuserModal } from './SuperuserModal';

type TabKey = 'dsn' | 'docker' | 'file';

const TABS: { key: TabKey; label: string; icon: typeof Database }[] = [
  { key: 'dsn', label: 'DSN', icon: Database },
  { key: 'docker', label: 'Docker', icon: Container },
  { key: 'file', label: 'File', icon: FileCode },
];

// Error codes that are meaningful enough to show inline on the form (plus toast).
const INLINE_CODES = new Set(['CONN_AUTH_FAILED', 'CONN_INVALID_DSN', 'CONN_UNREACHABLE']);

export function ConnectionWizard({
  onConnected,
}: {
  onConnected: (c: ConnMeta) => void;
}) {
  const [tab, setTab] = useState<TabKey>('dsn');
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  // SQL guidance shown when the backend rejects superuser credentials (§10.2).
  const [superuserSql, setSuperuserSql] = useState<string | null>(null);

  const submit = useCallback(
    async (config: ConnectionConfig) => {
      setSubmitting(true);
      setFormError(null);
      try {
        const meta = await connectionsApi.create(config);
        toast.success(`Connected to ${meta.config.label || meta.config.database || meta.engine}`);
        onConnected(meta);
      } catch (err) {
        if (err instanceof APIError) {
          if (err.code === 'CONN_SUPERUSER_REJECTED') {
            // Pull the create-role SQL from error.details.sql (§10.2). Fall back to
            // a generic message if the backend omitted it.
            const sql = extractSql(err.details);
            setSuperuserSql(sql ?? '-- No SQL guidance was provided by the server.');
            return;
          }
          const message = err.hint ? `${err.message} — ${err.hint}` : err.message;
          if (INLINE_CODES.has(err.code)) {
            setFormError(message);
          }
          toast.error(message);
        } else {
          const message = err instanceof Error ? err.message : 'Connection failed';
          setFormError(message);
          toast.error(message);
        }
      } finally {
        setSubmitting(false);
      }
    },
    [onConnected],
  );

  return (
    <div className="flex min-h-screen items-center justify-center bg-canvas-light p-8 text-neutral-900 dark:bg-canvas-dark dark:text-neutral-100">
      <div className="w-full max-w-xl rounded-lg border border-black/10 bg-white/60 shadow-sm dark:border-white/10 dark:bg-white/5">
        <div className="border-b border-black/10 px-6 pt-6 pb-4 dark:border-white/10">
          <h1 className="text-lg font-semibold">Connect a database</h1>
          <p className="mt-1 text-sm text-neutral-500">
            Read-only. Your credentials stay on this machine.
          </p>
        </div>

        {/* Tabs */}
        <div className="flex gap-1 px-6 pt-4" role="tablist" aria-label="Connection method">
          {TABS.map(({ key, label, icon: Icon }) => {
            const active = key === tab;
            return (
              <button
                key={key}
                type="button"
                role="tab"
                aria-selected={active}
                onClick={() => {
                  setTab(key);
                  setFormError(null);
                }}
                className={
                  'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors ' +
                  (active
                    ? 'bg-black/10 font-medium dark:bg-white/10'
                    : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
                }
              >
                <Icon size={15} aria-hidden />
                {label}
              </button>
            );
          })}
        </div>

        {/* Tab panels with a soft cross-fade (§13.4 — framer-motion for panels). */}
        <div className="px-6 pb-6 pt-4">
          <AnimatePresence mode="wait">
            <motion.div
              key={tab}
              initial={{ opacity: 0, y: 4 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -4 }}
              transition={{ duration: 0.15 }}
              role="tabpanel"
            >
              {tab === 'dsn' && (
                <DSNForm onSubmit={submit} busy={submitting} formError={formError} />
              )}
              {tab === 'docker' && (
                <DockerPicker onSubmit={submit} busy={submitting} formError={formError} />
              )}
              {tab === 'file' && (
                <FileForm onSubmit={submit} busy={submitting} formError={formError} />
              )}
            </motion.div>
          </AnimatePresence>
        </div>
      </div>

      <SuperuserModal sql={superuserSql} onClose={() => setSuperuserSql(null)} />
    </div>
  );
}

// extractSql reads the create-role SQL from an APIError.details payload of the
// shape { sql: string } (§10.2). Defensive against unexpected shapes.
function extractSql(details: unknown): string | null {
  if (details && typeof details === 'object' && 'sql' in details) {
    const sql = (details as { sql?: unknown }).sql;
    if (typeof sql === 'string') return sql;
  }
  return null;
}
