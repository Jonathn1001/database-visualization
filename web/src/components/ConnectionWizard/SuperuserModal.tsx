// Modal shown when the backend returns CONN_SUPERUSER_REJECTED (§8.3, §10.2).
// The tool refuses superuser credentials; this surfaces the create-role SQL from
// error.details.sql so the user can provision a least-privilege reader role.
import { useEffect, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { Check, Copy, ShieldAlert, X } from 'lucide-react';

export function SuperuserModal({ sql, onClose }: { sql: string | null; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const open = sql !== null;

  // Close on Escape while open.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  // Reset the copied state whenever a new modal opens.
  useEffect(() => {
    if (open) setCopied(false);
  }, [open]);

  const copy = async () => {
    if (!sql) return;
    try {
      await navigator.clipboard.writeText(sql);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard may be unavailable (insecure context); ignore — the SQL is selectable.
    }
  };

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-6"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.15 }}
          onClick={onClose}
          role="dialog"
          aria-modal="true"
          aria-labelledby="superuser-modal-title"
        >
          <motion.div
            className="w-full max-w-lg rounded-lg border border-black/10 bg-canvas-light text-neutral-900 shadow-lg dark:border-white/10 dark:bg-canvas-dark dark:text-neutral-100"
            initial={{ opacity: 0, scale: 0.97, y: 8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.97, y: 8 }}
            transition={{ duration: 0.15 }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-start gap-3 border-b border-black/10 px-5 py-4 dark:border-white/10">
              <ShieldAlert size={20} className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400" aria-hidden />
              <div className="min-w-0">
                <h2 id="superuser-modal-title" className="text-base font-semibold">
                  Superuser credentials refused
                </h2>
                <p className="mt-1 text-sm text-neutral-500">
                  dbviz is read-only and won’t connect as a superuser. Create a least-privilege
                  reader role, then reconnect with it.
                </p>
              </div>
              <button
                type="button"
                aria-label="Close"
                onClick={onClose}
                className="ml-auto rounded p-1 text-neutral-500 hover:bg-black/10 dark:hover:bg-white/10"
              >
                <X size={16} />
              </button>
            </div>

            <div className="px-5 py-4">
              <div className="relative">
                <pre className="max-h-64 overflow-auto rounded-md border border-black/10 bg-white/70 p-3 text-xs leading-relaxed dark:border-white/10 dark:bg-white/5">
                  <code className="font-mono whitespace-pre">{sql}</code>
                </pre>
                <button
                  type="button"
                  onClick={copy}
                  className="absolute right-2 top-2 inline-flex items-center gap-1 rounded border border-black/10 bg-canvas-light/90 px-2 py-1 text-xs hover:bg-black/5 dark:border-white/10 dark:bg-canvas-dark/90 dark:hover:bg-white/10"
                >
                  {copied ? <Check size={12} aria-hidden /> : <Copy size={12} aria-hidden />}
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>

            <div className="flex justify-end border-t border-black/10 px-5 py-3 dark:border-white/10">
              <button
                type="button"
                onClick={onClose}
                className="rounded-md bg-neutral-800 px-4 py-2 text-sm font-medium text-white hover:bg-neutral-700 dark:bg-neutral-200 dark:text-neutral-900 dark:hover:bg-white"
              >
                Got it
              </button>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
