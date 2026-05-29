// Renders children only on screens >= 1024px wide; otherwise shows the
// desktop-only message (DESIGN_PLAN §13.5). This is the single responsive gate
// in the app — no other responsive work is in scope.
import { useEffect, useState } from 'react';
import { DESKTOP_MEDIA_QUERY } from '@/lib/constants';

function matches(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return true;
  return window.matchMedia(DESKTOP_MEDIA_QUERY).matches;
}

export default function DesktopOnlyGate({ children }: { children: React.ReactNode }) {
  const [isDesktop, setIsDesktop] = useState<boolean>(matches);

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return;
    const mql = window.matchMedia(DESKTOP_MEDIA_QUERY);
    const onChange = () => setIsDesktop(mql.matches);
    // Sync immediately in case it changed between initial render and effect.
    onChange();
    mql.addEventListener('change', onChange);
    window.addEventListener('resize', onChange);
    return () => {
      mql.removeEventListener('change', onChange);
      window.removeEventListener('resize', onChange);
    };
  }, []);

  if (!isDesktop) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-2 bg-canvas-light px-6 text-center text-neutral-700 dark:bg-canvas-dark dark:text-neutral-300">
        <p className="text-lg font-semibold">DBViz is desktop-only.</p>
        <p className="text-sm">Please open on a screen at least 1024px wide.</p>
      </div>
    );
  }

  return <>{children}</>;
}
