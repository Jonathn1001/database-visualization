import { useEffect, useState } from 'react';

type Health = { status: string };

export default function App() {
  const [health, setHealth] = useState<'loading' | 'ok' | 'error'>('loading');

  useEffect(() => {
    fetch('/api/health')
      .then((r) => (r.ok ? (r.json() as Promise<Health>) : Promise.reject()))
      .then((d) => setHealth(d.status === 'ok' ? 'ok' : 'error'))
      .catch(() => setHealth('error'));
  }, []);

  return (
    <main className="min-h-screen bg-canvas-light dark:bg-canvas-dark text-neutral-900 dark:text-neutral-100 flex flex-col items-center justify-center gap-3">
      <h1 className="text-2xl font-semibold">DBViz</h1>
      <p className="text-sm opacity-70">Universal Database Visualizer</p>
      <p className="text-sm">
        backend:{' '}
        <span
          className={
            health === 'ok'
              ? 'text-green-600'
              : health === 'error'
                ? 'text-red-600'
                : 'opacity-50'
          }
        >
          {health === 'loading' ? 'checking…' : health === 'ok' ? 'connected' : 'unreachable'}
        </span>
      </p>
    </main>
  );
}
