// Query Path input: a small SQL editor + Run button. The CodeMirror editor
// (@uiw/react-codemirror) and its SQL language support (@codemirror/lang-sql)
// are heavy, so they are loaded lazily via React.lazy + Suspense and are kept
// OUT of the initial bundle (DESIGN_PLAN §14.3). Calls onSubmit(sql) on Run.
import { lazy, Suspense, useState } from 'react';
import { Play } from 'lucide-react';

// Lazily-loaded editor. React.lazy needs a module whose `default` export is a
// React component, so we wrap CodeMirror + the SQL dialect into a tiny module
// resolved on first use. Neither package is referenced anywhere eagerly, so the
// bundler splits them into a separate chunk.
const LazySqlEditor = lazy(async () => {
  const [{ default: CodeMirror }, { sql, PostgreSQL }] = await Promise.all([
    import('@uiw/react-codemirror'),
    import('@codemirror/lang-sql'),
  ]);

  function SqlEditor({
    value,
    onChange,
    onRun,
  }: {
    value: string;
    onChange: (v: string) => void;
    onRun: () => void;
  }) {
    return (
      <CodeMirror
        value={value}
        height="80px"
        theme="light"
        extensions={[sql({ dialect: PostgreSQL })]}
        onChange={onChange}
        basicSetup={{
          lineNumbers: false,
          foldGutter: false,
          highlightActiveLine: false,
          highlightActiveLineGutter: false,
        }}
        placeholder="SELECT * FROM users JOIN orders ON ..."
        onKeyDownCapture={(e) => {
          // Cmd/Ctrl+Enter runs the query.
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            onRun();
          }
        }}
      />
    );
  }

  return { default: SqlEditor };
});

export function QueryPathInput({
  onSubmit,
  busy,
}: {
  onSubmit: (sql: string) => void;
  busy?: boolean;
}) {
  const [sqlText, setSqlText] = useState('');

  const trimmed = sqlText.trim();
  const canRun = trimmed.length > 0 && !busy;

  function run() {
    if (trimmed.length === 0 || busy) return;
    onSubmit(trimmed);
  }

  return (
    <div className="flex h-full flex-col gap-2">
      <div className="min-h-0 flex-1 overflow-auto rounded border border-black/10 bg-white/60 text-[13px] dark:border-white/10 dark:bg-white/5">
        <Suspense
          fallback={
            <div className="flex h-full items-center px-2 py-2 text-xs text-neutral-500">
              Loading editor…
            </div>
          }
        >
          <LazySqlEditor value={sqlText} onChange={setSqlText} onRun={run} />
        </Suspense>
      </div>
      <div className="flex shrink-0 items-center justify-between">
        <span className="text-[11px] text-neutral-500">
          EXPLAIN only · read-only · ⌘/Ctrl+Enter to run
        </span>
        <button
          type="button"
          onClick={run}
          disabled={!canRun}
          className="inline-flex items-center gap-1.5 rounded border border-black/10 bg-white/70 px-2.5 py-1 text-xs font-medium hover:bg-black/5 disabled:cursor-not-allowed disabled:opacity-40 dark:border-white/10 dark:bg-white/10 dark:hover:bg-white/15"
        >
          <Play size={13} />
          {busy ? 'Running…' : 'Run'}
        </button>
      </div>
    </div>
  );
}
