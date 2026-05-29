// Small presentational helpers shared by the wizard tabs. Kept local to the
// ConnectionWizard so the styling (muted, paper-like, §13.4) stays consistent
// across the DSN / Docker / File forms without pulling in a UI primitive library.
import { forwardRef } from 'react';
import type { ReactNode } from 'react';
import { CircleAlert, CircleCheck, LoaderCircle } from 'lucide-react';

export const inputClass =
  'w-full rounded-md border border-black/15 bg-white/70 px-3 py-2 text-sm text-neutral-900 ' +
  'placeholder:text-neutral-400 outline-none focus:border-black/40 ' +
  'dark:border-white/15 dark:bg-white/5 dark:text-neutral-100 dark:focus:border-white/40';

export function FieldLabel({ children }: { children: ReactNode }) {
  return <label className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-300">{children}</label>;
}

/** A monospace text input that forwards its ref for react-hook-form register(). */
export const TextField = forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  function TextField(props, ref) {
    return <input ref={ref} {...props} className={`${inputClass} ${props.className ?? ''}`} />;
  },
);

/** Inline field-level validation message (red, small). */
export function FieldError({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <p className="mt-1 flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
      <CircleAlert size={12} aria-hidden />
      {message}
    </p>
  );
}

/** Form-level error banner for server-side connection failures (§8.3). */
export function FormErrorBanner({ message }: { message?: string | null }) {
  if (!message) return null;
  return (
    <div className="flex items-start gap-2 rounded-md border border-red-300/60 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/40 dark:bg-red-950/40 dark:text-red-300">
      <CircleAlert size={15} className="mt-0.5 shrink-0" aria-hidden />
      <span>{message}</span>
    </div>
  );
}

/** Result of the POST /test dry-run shown beneath the DSN field. */
export function TestResult({
  state,
}: {
  state:
    | { kind: 'idle' }
    | { kind: 'testing' }
    | { kind: 'ok'; engine: string }
    | { kind: 'error'; message: string };
}) {
  if (state.kind === 'idle') return null;
  if (state.kind === 'testing') {
    return (
      <p className="mt-1.5 flex items-center gap-1.5 text-xs text-neutral-500">
        <LoaderCircle size={12} className="animate-spin" aria-hidden />
        Testing connection…
      </p>
    );
  }
  if (state.kind === 'ok') {
    return (
      <p className="mt-1.5 flex items-center gap-1.5 text-xs text-green-700 dark:text-green-400">
        <CircleCheck size={12} aria-hidden />
        Reachable — {state.engine}
      </p>
    );
  }
  return (
    <p className="mt-1.5 flex items-center gap-1.5 text-xs text-red-600 dark:text-red-400">
      <CircleAlert size={12} aria-hidden />
      {state.message}
    </p>
  );
}

/** Primary submit button with a busy spinner. */
export function SubmitButton({
  busy,
  disabled,
  children,
}: {
  busy?: boolean;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      type="submit"
      disabled={busy || disabled}
      className="inline-flex items-center justify-center gap-2 rounded-md bg-neutral-800 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-neutral-200 dark:text-neutral-900 dark:hover:bg-white"
    >
      {busy && <LoaderCircle size={14} className="animate-spin" aria-hidden />}
      {children}
    </button>
  );
}
