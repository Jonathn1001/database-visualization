// Shared error-UX hook (DESIGN_PLAN §8.3). Returns a stable callback that maps
// an APIError (its .code) to the right user-facing reaction:
//   - a sonner toast with code-specific copy + optional backend hint
//   - for CONN_NOT_FOUND, clears the connection store so App falls back to the
//     wizard ("session expired"), per §19.2 (closed) / §8.3 (redirect to wizard)
//
// Any non-APIError (network blip, thrown string, etc.) gets a generic toast.
// Export is used by App and by feature components (e.g. ConnectionStatus,
// ActionBar) so error handling stays consistent across the app.
import { useCallback } from 'react';
import { toast } from 'sonner';

import { APIError } from '@/api/client';
import { useConnectionStore } from '@/store/connection';
import type { ErrorCode } from '@/types/graph';

// Human copy per error code (§8.3). Kept terse and calm to match the visual
// language; the backend `hint` (when present) is shown as the toast description.
const MESSAGES: Record<ErrorCode, string> = {
  CONN_INVALID_DSN: 'Invalid connection string.',
  CONN_AUTH_FAILED: 'Authentication failed.',
  CONN_UNREACHABLE: 'Database is unreachable.',
  CONN_SUPERUSER_REJECTED: 'Superuser connections are rejected.',
  CONN_NOT_FOUND: 'Session expired, please reconnect.',
  CONN_TIMEOUT: 'The connection timed out.',
  CONN_ALREADY_CLOSED: 'This connection is already closed.',
  SCHEMA_INTROSPECT_FAILED: 'Could not read the database schema.',
  SCHEMA_PERMISSION_DENIED: 'Permission denied while reading the schema.',
  SCHEMA_NOT_FOUND: 'Schema not found.',
  SCHEMA_TOO_LARGE: 'Schema is too large — filter schemas to continue.',
  EXPLAIN_INVALID_SQL: 'That query could not be parsed.',
  EXPLAIN_NOT_SUPPORTED: 'EXPLAIN is not supported for this connection.',
  EXPLAIN_TIMEOUT: 'The query plan timed out.',
  ADAPTER_NOT_SUPPORTED: 'This database engine is not supported.',
  ADAPTER_OPERATION_NOT_SUPPORTED: 'This action is not available for this engine.',
  ADAPTER_READONLY_VIOLATION: 'This tool is read-only; that action was blocked.',
  INTERNAL: 'Something went wrong on the server.',
  BAD_REQUEST: 'The request was rejected.',
  RATE_LIMITED: 'Too many requests — slow down a moment.',
};

const GENERIC_MESSAGE = 'Something went wrong.';

/** A code is "known" if we have curated copy for it. */
function isErrorCode(code: string): code is ErrorCode {
  return Object.prototype.hasOwnProperty.call(MESSAGES, code);
}

/**
 * useApiErrorHandler returns a stable `handle(err)` callback that surfaces the
 * appropriate UX for any error thrown by the api client. Callers can pass any
 * unknown caught value; non-APIError values get a generic toast.
 */
export function useApiErrorHandler() {
  const clearConn = useConnectionStore((s) => s.clearConn);

  return useCallback(
    (err: unknown): void => {
      if (err instanceof APIError) {
        const message = isErrorCode(err.code) ? MESSAGES[err.code] : err.message || GENERIC_MESSAGE;

        // CONN_NOT_FOUND: the server no longer knows this connection. Drop it
        // from the store so App returns to the wizard (§8.3, §19.2 "closed").
        if (err.code === 'CONN_NOT_FOUND') {
          clearConn();
          toast.error(message);
          return;
        }

        toast.error(message, err.hint ? { description: err.hint } : undefined);
        return;
      }

      toast.error(GENERIC_MESSAGE);
    },
    [clearConn],
  );
}
