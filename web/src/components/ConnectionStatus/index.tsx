// ConnectionStatus — passive health indicator for the active connection
// (DESIGN_PLAN §19). Polls POST /api/connections/:id/ping on an interval and
// maps the query lifecycle to four states:
//
//   connected | green dot, subtle           | normal
//   degraded  | yellow dot, "Reconnecting…" | a ping failed; React Query retrying
//   lost      | red dot, persistent toast   | retries exhausted; "Reconnect" button
//   closed    | (none)                      | server says CONN_NOT_FOUND ->
//                                             clear the connection store so App
//                                             returns to the wizard + toast
//
// This component only surfaces state via the dot/toast. It does NOT dim the graph
// or disable global actions (App / ActionBar own that, reading nothing from here).
import { useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { RefreshCw } from 'lucide-react';

import * as connectionsApi from '@/api/connections';
import { APIError } from '@/api/client';
import { useConnectionStore } from '@/store/connection';

// §19.1: refetch every 30s, retry 3 times with exponential backoff capped at 10s.
const PING_REFETCH_INTERVAL_MS = 30_000;
const PING_RETRY = 3;
const PING_RETRY_DELAY = (attempt: number) => Math.min(1000 * 2 ** attempt, 10_000);

const LOST_TOAST_ID = 'connection-lost';

type Status = 'connected' | 'degraded' | 'lost' | 'closed';

export function ConnectionStatus({ connId }: { connId: string }) {
  const clearConn = useConnectionStore((s) => s.clearConn);

  const pingQuery = useQuery({
    queryKey: ['ping', connId],
    queryFn: () => connectionsApi.ping(connId),
    refetchInterval: PING_REFETCH_INTERVAL_MS,
    // CONN_NOT_FOUND means the connection is gone for good — don't burn retries.
    retry: (failureCount, error) => {
      if (error instanceof APIError && error.code === 'CONN_NOT_FOUND') return false;
      return failureCount < PING_RETRY;
    },
    retryDelay: PING_RETRY_DELAY,
  });

  const isClosed =
    pingQuery.error instanceof APIError && pingQuery.error.code === 'CONN_NOT_FOUND';

  let status: Status;
  if (isClosed) {
    status = 'closed';
  } else if (pingQuery.isError) {
    status = 'lost';
  } else if (pingQuery.isSuccess && pingQuery.failureCount === 0) {
    status = 'connected';
  } else if (pingQuery.failureCount > 0) {
    // A ping failed and React Query is retrying (or about to). Treat any
    // in-flight failure state before exhaustion as degraded.
    status = 'degraded';
  } else {
    // Initial load before the first ping resolves: stay quiet (assume connected).
    status = 'connected';
  }

  // Closed: the server forgot this connection. Clear the store (App -> wizard)
  // and tell the user once. Guarded so it fires a single time per drop.
  const closedFiredRef = useRef(false);
  useEffect(() => {
    if (status === 'closed' && !closedFiredRef.current) {
      closedFiredRef.current = true;
      toast.dismiss(LOST_TOAST_ID);
      toast.error('Session expired, please reconnect.');
      clearConn();
    }
    if (status !== 'closed') {
      closedFiredRef.current = false;
    }
  }, [status, clearConn]);

  // Keep the latest refetch in a ref so the toast effect can depend only on
  // `status` (the toast should fire on transitions, not on every refetch churn).
  const refetchRef = useRef(pingQuery.refetch);
  refetchRef.current = pingQuery.refetch;

  // Lost: show a persistent toast with a Reconnect action (re-runs the ping
  // query). Dismiss it when we recover.
  useEffect(() => {
    if (status === 'lost') {
      toast.error('Connection lost.', {
        id: LOST_TOAST_ID,
        duration: Infinity,
        description: 'Could not reach the database after several attempts.',
        action: {
          label: 'Reconnect',
          onClick: () => {
            void refetchRef.current();
          },
        },
      });
    } else {
      toast.dismiss(LOST_TOAST_ID);
    }
  }, [status]);

  // Tidy up the persistent toast if this component unmounts (e.g. disconnect).
  useEffect(() => {
    return () => {
      toast.dismiss(LOST_TOAST_ID);
    };
  }, []);

  if (status === 'closed') return null;

  const dot =
    status === 'connected'
      ? { color: 'bg-emerald-500', title: 'Connected' }
      : status === 'degraded'
        ? { color: 'bg-amber-500', title: 'Reconnecting…' }
        : { color: 'bg-red-500', title: 'Connection lost' };

  return (
    <span className="inline-flex items-center gap-1.5" title={dot.title}>
      <span
        className={`inline-block h-2 w-2 rounded-full ${dot.color} ${
          status === 'degraded' ? 'animate-pulse' : ''
        }`}
        aria-hidden="true"
      />
      {status === 'degraded' && (
        <span className="text-xs text-amber-600 dark:text-amber-400">Reconnecting…</span>
      )}
      {status === 'lost' && (
        <button
          type="button"
          onClick={() => void pingQuery.refetch()}
          className="inline-flex items-center gap-1 rounded border border-red-500/40 px-1.5 py-0.5 text-xs text-red-600 transition-colors hover:bg-red-500/10 dark:text-red-400"
        >
          <RefreshCw size={12} className={pingQuery.isFetching ? 'animate-spin' : ''} />
          Reconnect
        </button>
      )}
      <span className="sr-only">{dot.title}</span>
    </span>
  );
}
