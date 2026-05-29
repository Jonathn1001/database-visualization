// SchemaPicker — top-left multi-select chip dropdown of loaded schemas
// (DESIGN_PLAN §11.3). `current` is what is loaded now (empty = adapter default,
// usually "public"); `available` is everything from GET /schemas. Toggling an
// item calls onChange(newSchemas); App refetches the schema graph in response.
//
// This component is purely about selection. The SCHEMA_FILTERED warning banner
// is rendered by App, not here.
import { useEffect, useRef, useState } from 'react';
import { ChevronDown, Check, Layers } from 'lucide-react';

export function SchemaPicker({
  connId,
  current,
  available,
  onChange,
}: {
  connId: string;
  current: string[];
  available: string[];
  onChange: (schemas: string[]) => void;
}) {
  void connId; // selection is keyed off props; connId is contextual only.
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  // Close on outside click / Escape.
  useEffect(() => {
    if (!open) return;
    function onPointerDown(e: PointerEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  const selected = new Set(current);

  function toggle(schema: string) {
    const next = new Set(selected);
    if (next.has(schema)) {
      next.delete(schema);
    } else {
      next.add(schema);
    }
    // Preserve the `available` ordering for a stable, predictable result.
    onChange(available.filter((s) => next.has(s)));
  }

  // Label summarizes the active selection. Empty current = adapter default.
  const label =
    current.length === 0
      ? 'Default schema'
      : current.length === 1
        ? current[0]
        : `${current.length} schemas`;

  const disabled = available.length === 0;

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className="flex items-center gap-1.5 rounded border border-black/10 bg-white/40 px-2.5 py-1 text-sm text-neutral-700 transition-colors hover:bg-black/5 disabled:cursor-default disabled:opacity-50 dark:border-white/10 dark:bg-white/5 dark:text-neutral-200 dark:hover:bg-white/10"
      >
        <Layers size={14} className="opacity-60" />
        <span className="max-w-[12rem] truncate">{label}</span>
        <ChevronDown
          size={14}
          className={`opacity-60 transition-transform ${open ? 'rotate-180' : ''}`}
        />
      </button>

      {/* Selected schemas shown as inline chips next to the trigger. */}
      {current.length > 0 && (
        <span className="ml-2 hidden items-center gap-1 align-middle lg:inline-flex">
          {current.slice(0, 3).map((s) => (
            <span
              key={s}
              className="rounded-full bg-black/5 px-2 py-0.5 text-xs text-neutral-600 dark:bg-white/10 dark:text-neutral-300"
            >
              {s}
            </span>
          ))}
          {current.length > 3 && (
            <span className="text-xs text-neutral-500">+{current.length - 3}</span>
          )}
        </span>
      )}

      {open && (
        <div
          role="listbox"
          aria-multiselectable="true"
          className="absolute left-0 top-full z-20 mt-1 max-h-72 w-56 overflow-auto rounded-md border border-black/10 bg-canvas-light py-1 shadow-lg dark:border-white/10 dark:bg-canvas-dark"
        >
          {available.map((schema) => {
            const isSelected = selected.has(schema);
            return (
              <button
                key={schema}
                type="button"
                role="option"
                aria-selected={isSelected}
                onClick={() => toggle(schema)}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-neutral-700 transition-colors hover:bg-black/5 dark:text-neutral-200 dark:hover:bg-white/10"
              >
                <span
                  className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border ${
                    isSelected
                      ? 'border-transparent bg-neutral-700 text-white dark:bg-neutral-200 dark:text-neutral-900'
                      : 'border-black/20 dark:border-white/20'
                  }`}
                >
                  {isSelected && <Check size={12} strokeWidth={3} />}
                </span>
                <span className="truncate">{schema}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
