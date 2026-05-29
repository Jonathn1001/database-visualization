// Client-side engine auto-detection from a DSN/URI or file path (§13.3). The
// backend remains the source of truth (POST /test confirms the real engine); this
// is only a hint shown in the wizard so the user gets immediate feedback.
import type { Engine } from '@/types/graph';

// Default ports per engine, used to prefill Docker-derived connections (§13.3).
export const DEFAULT_PORTS: Record<Engine, number> = {
  postgres: 5432,
  mongodb: 27017,
  sqlite: 0, // file-based, no port
};

/**
 * detectEngineFromDSN inspects a DSN/URI string and returns the detected engine,
 * or null if it can't be determined yet. Recognizes:
 *   postgres:// , postgresql://      -> postgres
 *   mongodb:// , mongodb+srv://      -> mongodb
 *   sqlite:// , file:// , *.db/.sqlite/.sqlite3 -> sqlite
 */
export function detectEngineFromDSN(raw: string): Engine | null {
  const dsn = raw.trim();
  if (dsn === '') return null;

  const lower = dsn.toLowerCase();
  if (lower.startsWith('postgres://') || lower.startsWith('postgresql://')) {
    return 'postgres';
  }
  if (lower.startsWith('mongodb://') || lower.startsWith('mongodb+srv://')) {
    return 'mongodb';
  }
  if (lower.startsWith('sqlite://') || lower.startsWith('file:')) {
    return 'sqlite';
  }
  if (isSqliteFilePath(lower)) {
    return 'sqlite';
  }
  return null;
}

/** isSqliteFilePath returns true for paths ending in a SQLite file extension. */
export function isSqliteFilePath(pathOrDsn: string): boolean {
  const lower = pathOrDsn.toLowerCase();
  return (
    lower.endsWith('.db') ||
    lower.endsWith('.sqlite') ||
    lower.endsWith('.sqlite3')
  );
}

/** Human-readable label for a detected engine. */
export function engineLabel(engine: Engine | null): string {
  switch (engine) {
    case 'postgres':
      return 'PostgreSQL';
    case 'mongodb':
      return 'MongoDB';
    case 'sqlite':
      return 'SQLite';
    default:
      return 'Unknown';
  }
}
