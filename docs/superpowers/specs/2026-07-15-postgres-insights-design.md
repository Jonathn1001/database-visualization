# Deeper Postgres Insights — Design

**Date:** 2026-07-15
**Status:** Approved (brainstorm session)
**Sub-project:** 1 of 4 (queued after this: more engines → richer graph UX → live features)

## Goal

Add analysis features on top of the existing Postgres adapter that surface actionable
schema/health findings in the graph UI: index health, relationship gaps, table health,
and schema snapshots with diff. Catalog metadata only — no queries against user row data.

**Relation to DESIGN_PLAN.md:** this implements three items named in DESIGN_PLAN §1.5
(Phase 2 scope): *index utilization x-ray* → index health, *bloat visualizer* → table
health, *schema evolution timeline* → snapshots + diff. Relationship gaps is new but
reuses the inferred-edge contract DESIGN_PLAN §2.3 defined for NoSQL heuristics.

## Scope

**In (v1):**
- Index health: unused indexes, duplicate/redundant indexes, missing indexes on FK columns.
- Relationship gaps: columns that look like foreign keys but have no constraint —
  catalog heuristics only (name + type match), no data sampling.
- Table health: dead-tuple ratio, bloat estimate, last vacuum/analyze, seq-scan-heavy tables.
- Schema snapshots persisted locally + diff between any two snapshots (or snapshot vs current).
- UI: insights panel (sortable findings, per-category tabs) + graph overlay
  (severity badges on nodes, dashed implied-FK edges, click-to-highlight).

**Out (v1):**
- Data-sampling verification of FK candidates (possible later as opt-in per candidate).
- Insights for non-Postgres engines (gaps + diff work engine-agnostically by design;
  MongoDB/SQLite adapters are scoped by DESIGN_PLAN §1.4 but not yet implemented).
- Automatic/scheduled snapshots. Insights run **on demand only** (panel open / refresh
  click) — never automatically after introspect.

## Architecture

Chosen approach: **insights package + optional capability interfaces** (over extending the
core `Adapter` interface, or a SQL-driven insight registry). Mirrors the existing
`internal/simulate` pattern: pure functions over `model.GraphModel`, engine specifics
behind narrow interfaces. Core `Adapter` interface stays untouched.

### New package `internal/insights`

| File | Contents | Purity |
|---|---|---|
| `gaps.go` | `RelationshipGaps(g *model.GraphModel) []Finding` | pure |
| `diff.go` | `Diff(old, new *model.GraphModel) SchemaDiff` | pure |
| `index.go` | `IndexFindings(stats []IndexStat, g *model.GraphModel) []Finding` | pure logic; data via capability |
| `health.go` | `HealthFindings(stats []model.TableStats) []Finding` | pure logic; data via capability |

**Gap heuristics:** column named `<table>_id` or `<singular-of-table>_id` whose type matches
the target table's PK type, with no existing FK constraint. Each candidate carries a
confidence score: exact name+type match scores higher than suffix-only match.

**Implied edges reuse the existing model contract** (DESIGN_PLAN §2.3): gap candidates
emit `model.Link{Inferred: true, Confidence: …}` — the same fields built for NoSQL
heuristic edges (`internal/model/graph.go`, `Link.Inferred`/`FKRef.Inferred`). No new
edge shape. The frontend dashed renderer keys off `link.inferred`, so future MongoDB
inferred links get the same rendering free.

**Diff output:** added/dropped tables, columns, FK constraints; column type changes;
added/dropped indexes **by name only** — `Column.Indexes` is `[]string`, so index
definition changes (e.g. column list, uniqueness) are out of diff's reach in v1.

### Capability interfaces (declared in `internal/insights`)

```go
type IndexStatser interface {
    IndexStats(ctx context.Context) ([]IndexStat, error)   // pg_stat_user_indexes + pg_index
}
type BulkTableStatser interface {
    AllTableStats(ctx context.Context) ([]model.TableStats, error) // pg_stat_user_tables, one query
}
```

- **No parallel stats type.** `BulkTableStatser` returns the existing `model.TableStats`
  (already carries dead tuples, vacuum/analyze timestamps, ins/upd/del — see
  `internal/adapter/postgres/stats.go`). Implementation is a bulk variant of the existing
  `queryTableStats` SQL, sharing the row-scan logic with the per-table `TableStats` method.
- `model.TableStats` gains two fields for the seq-scan-heavy finding: `SeqScans` and
  `IdxScans` (from `pg_stat_user_tables.seq_scan` / `idx_scan`). Both `omitempty`, so the
  existing `/tables/{nodeId}/stats` response stays backward-compatible.
- Postgres adapter implements both capabilities in new file `internal/adapter/postgres/insights.go`.
- The insights handler type-asserts the adapter; a missing capability makes that category
  report `status: "unsupported"` — never an error.
- `RelationshipGaps` and `Diff` need only `GraphModel`, so any future engine gets them free.

### Snapshot store — new package `internal/snapshot`

- Location: `~/.dbviz/snapshots/<fingerprint>/<timestamp>.json`.
- `fingerprint` = SHA-256 of the redacted DSN (host + port + database, **no credentials**),
  so snapshots survive connection recreation and no secrets touch disk.
- Snapshot file = serialized `GraphModel` + metadata `{taken_at, engine, dbviz_version}`.
- Prune policy: keep last N per fingerprint (default 20, env `DBVIZ_SNAPSHOT_KEEP`
  following the §7.3 env-var convention), oldest deleted on write.

**Persistence posture — deliberate amendment.** dbviz has until now persisted nothing
across restarts: connection IDs are server-side ephemeral (DESIGN_PLAN §7.4) and open
question #1 records "re-enter every session for security" as a conscious decision.
Snapshots are the project's **first persistent on-disk artifact**. This spec amends that
posture for non-credential data only:

- **Stored:** schema structure (table/column/index names, types, FK constraints), row
  counts, size bytes, timestamps, engine name, dbviz version.
- **Never stored:** credentials, DSNs (only their one-way hash), row data, sample data,
  query text.

The original posture stands unchanged for connections: they remain ephemeral,
credentials are re-entered every session.

## API surface

```
GET    /api/connections/{id}/insights                   all categories; ?category=index|health|gaps
POST   /api/connections/{id}/snapshots                  take snapshot (introspect + persist)
GET    /api/connections/{id}/snapshots                  list {id, taken_at, table_count}
GET    /api/connections/{id}/snapshots/diff?from=&to=   SchemaDiff; to=current allowed
DELETE /api/connections/{id}/snapshots/{snapId}         delete one snapshot
```

- `/insights` wrapped in `timeout(introspectTimeout)` (same as `/schema`) — catalog queries only.
- `/snapshots/diff` also wrapped in `timeout(introspectTimeout)`: `to=current` runs a live
  introspect.
- Finding shape: `{category, severity: info|warn|critical, node_id, title, detail, meta}`.
  `node_id` keys the graph highlight.
- Unsupported capability → `{category, status: "unsupported"}` inside a 200 payload;
  frontend hides that category.
- Errors flow through the existing `NewAPIError` / `writeError` path. New codes, following
  §8 naming: `ErrSnapshotNotFound` (404 — unknown snapshot id in diff/delete) and
  `ErrSnapshotDiffInvalid` (400 — `from`/`to` missing, identical, or malformed).
- Snapshot create/delete recorded via existing `internal/audit` logger.

## Frontend

- **`web/src/components/InsightsPanel/`** — new side panel. Tabs per category plus a
  Snapshots tab. Findings sortable by severity and by table. Clicking a finding selects the
  node via `useSelectionStore`; canvas pans/highlights using existing selection mechanics.
- **Graph overlay** — `nodeRenderer.ts` gains a severity badge dot on affected nodes;
  `linkRenderer.ts` renders implied-FK edges dashed in a distinct color from `lib/colors.ts`.
  Overlay toggle lives in the ActionBar.
- **Snapshots tab** — take / list / delete; select two snapshots → diff view (grouped
  added/removed/changed list); changed nodes flagged on canvas.
- Data layer: TanStack Query hooks in new `web/src/api/insights.ts`, mirroring
  `api/schema.ts` conventions.

## Security & privacy

- Catalog metadata only; the feature never reads user row data (PII masking not implicated).
- Capability queries run on the adapter's existing pool, inheriting the read-only
  enforcement of DESIGN_PLAN §10.2.
- Snapshot files contain schema structure only, never credentials or rows. One caveat:
  `Column.DefaultVal` is part of `GraphModel` and lands in snapshot files — column
  defaults can occasionally embed literals an operator considers sensitive. Accepted for
  v1; defaults are already exposed via the `/schema` endpoint today.
- DSN fingerprint is one-way (SHA-256) and excludes credentials.

## Testing

- Pure functions (`gaps`, `diff`, index/health logic): table-driven unit tests, same style
  as `internal/simulate/cascade_test.go`.
- Postgres capability queries: integration tests in the `adapter_integration_test.go`
  harness — fixture DB seeded with a known unused index and an FK-less `user_id` column.
- Snapshot store: tmpdir unit tests covering write/list/prune and fingerprint stability.
- Handlers: `api_integration_test.go` pattern — happy path, unsupported-capability
  degradation, invalid snapshot ids.

## Phasing (one PR each)

1. **Insights core:** `internal/insights` (gaps/index/health) + capability impls in the
   Postgres adapter + `/insights` endpoint + InsightsPanel + graph overlay.
2. **Snapshots:** `internal/snapshot` store + diff + snapshot endpoints + Snapshots tab
   + diff view.

## Open questions

- **Spec location vs `.gitignore`:** the repo's `.gitignore` has a bare `docs` entry, so
  this spec (and `DESIGN_PLAN.md`) are untracked by default; this file was force-added.
  Decide: narrow the ignore (e.g. keep `docs/superpowers/` tracked) or keep docs private.
- **Snapshot retention for the timeline feature:** keep-last-20 suffices for diff v1, but
  if snapshots grow into the §1.5 "schema evolution timeline", a count-based prune may
  discard history worth keeping. Revisit when timeline UI is designed.
