// Shared fetch wrapper. All API calls in the app go through `api<T>()` so error
// shapes ({error, code, details, hint}) are parsed consistently and surfaced as
// APIError instances (DESIGN_PLAN §8.3). Paths are RELATIVE ("/api/...") so the
// Vite dev proxy and the embedded prod server both resolve them correctly.

export class APIError extends Error {
  code: string;
  details?: unknown;
  hint?: string;

  constructor(code: string, message: string, details?: unknown, hint?: string) {
    super(message);
    this.name = 'APIError';
    this.code = code;
    this.details = details;
    this.hint = hint;
  }
}

interface ErrorBody {
  error?: string;
  code?: string;
  details?: unknown;
  hint?: string;
}

/**
 * api performs a fetch and normalizes the response:
 * - On non-2xx, parses the canonical {error, code, details, hint} body and
 *   throws an APIError.
 * - On 204 / empty body, resolves to undefined (cast to T for void responses).
 * - Otherwise parses and returns JSON.
 */
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      // Only attach a JSON content-type when there is a body to send.
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  });

  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as ErrorBody;
    throw new APIError(
      body.code ?? 'UNKNOWN',
      body.error ?? res.statusText,
      body.details,
      body.hint,
    );
  }

  // 204 No Content (e.g. DELETE) or any empty body.
  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  if (text.length === 0) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

/**
 * encodeNodeId URL-encodes a fully-qualified node id ("public.users") for use
 * in a path param. encodeURIComponent does NOT encode ".", so we replace it
 * explicitly with %2E as required by the API (§11.1).
 */
export function encodeNodeId(id: string): string {
  return encodeURIComponent(id).replace(/\./g, '%2E');
}
