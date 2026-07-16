# Postgres Insights Core (Phase ①) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the on-demand insights pipeline — index health, table health, relationship gaps — from pure Go analysis functions through a `GET /api/connections/{id}/insights` endpoint to an Insights side panel and a graph overlay (severity badges + dashed implied-FK edges).

**Architecture:** New package `internal/insights` holds pure functions over `model.GraphModel` (mirroring `internal/simulate`) plus narrow capability interfaces (`IndexStatser`, `BulkTableStatser`) that the Postgres adapter implements in a new file. The HTTP handler introspects, type-asserts capabilities, and degrades missing ones to `status:"unsupported"` inside a 200 payload. The frontend adds a TanStack-Query-backed InsightsPanel, a zustand insights store, and an overlay that reuses the existing inferred-link dashed renderer.

**Tech Stack:** Go 1.25.8 (chi, pgx/v5, testcontainers), React 18 + TypeScript + zustand + TanStack Query + d3, vitest.

**Spec:** `docs/superpowers/specs/2026-07-15-postgres-insights-design.md` (phase ① only — snapshots/diff are phase ②, a separate PR).

## Global Constraints

- Branch: `feat/pg-insights` (already checked out; spec commits `84b06ed`, `b0c0955` live here). One PR for this whole plan.
- Go module path: `github.com/elgnas/dbviz`. Go version: 1.25.8.
- **Toolchain workaround:** plain `go build` / `go vet` / `go test -short` use the system `go`. Full test runs (testcontainers integration) must use the downloaded toolchain binary or they can hit the 1.25.6/1.25.8 skew:
  `GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go test -count=1 ./...`
- Integration tests are guarded by `if testing.Short() { t.Skip(...) }` — always add this guard to new integration tests.
- **No new dependencies**, backend or frontend.
- JSON is camelCase everywhere (`nodeId`, not `node_id`). The spec sketches `node_id` in prose, but the frozen contract (`web/src/types/graph.ts` header: field names MUST match Go JSON tags; existing `TableStats.nodeId`, `CascadeStep.nodeId`) mandates camelCase. Use `nodeId`.
- Insights run **on demand only** — never automatically after introspect. Catalog metadata only; no user row data is read.
- Never stage: `internal/server/dist/*` (the pre-existing `D internal/server/dist/.gitkeep` in git status is NOT yours — leave it unstaged), `graphify-out/`, `internal/adapter/postgres/pure_test.go` (untracked, predates this work), docker-compose/config/.env files.
- `docs/` is gitignored via a bare `docs` entry — commit doc files with `git add -f`.
- Commit specific files, never `git add -A`. Message style: `feat: …` / `test: …` / `docs: …` (see `git log`).
- Frontend: lucide-react is pinned to the 1.x line by the registry mirror — before using a new icon, confirm it exists in `web/node_modules/lucide-react/dist/lucide-react.d.ts`; fall back to `Eye` if `Lightbulb` is absent.
- Frontend checks: `npm --prefix web run test` (vitest), `npm --prefix web run lint`, `npm --prefix web run build` (runs `tsc --noEmit` then vite build; the vite output lands in gitignored `internal/server/dist/`).

## File Structure

```
internal/insights/insights.go        # package doc, Finding/CategoryResult/Result, constants,
                                     #   IndexStat, IndexStatser, BulkTableStatser, sortFindings
internal/insights/insights_test.go   # sortFindings ordering test
internal/insights/gaps.go            # RelationshipGaps (pure)
internal/insights/gaps_test.go
internal/insights/index.go           # IndexFindings (pure)
internal/insights/index_test.go
internal/insights/health.go          # HealthFindings (pure)
internal/insights/health_test.go
internal/insights/collect.go         # Collect: orchestration + capability degradation
internal/insights/collect_test.go
internal/model/graph.go              # TableStats += SeqScans/IdxScans
internal/adapter/postgres/stats.go   # shared row-scan; per-table query gains seq/idx scans
internal/adapter/postgres/insights.go            # IndexStats + AllTableStats capability impls
internal/adapter/postgres/adapter_integration_test.go  # + capability integration test, count bumps
internal/server/handlers.go          # getInsights handler
internal/server/server.go            # route
internal/server/api_integration_test.go          # + endpoint integration test, count bump
internal/audit/log.go                # OpInsights const
testdata/ecommerce.sql               # + audit_logs table, + duplicate index

web/src/types/graph.ts               # Finding/InsightCategoryResult/InsightsResult, TableStats fields
web/src/api/insights.ts              # getInsights()
web/src/store/insights.ts            # result + overlay zustand store
web/src/lib/insights.ts              # severityRank/sortFindings/impliedLinks/mergeImpliedLinks/severityByNode
web/src/lib/insights.test.ts         # vitest for the pure helpers
web/src/components/InsightsPanel/index.tsx        # side panel
web/src/App.tsx                      # aside tabs (Inspector | Insights), merged display graph
web/src/lib/colors.ts                # severityColor()
web/src/components/Graph/nodeRenderer.ts          # renderSeverityBadges()
web/src/components/Graph/GraphCanvas.tsx          # badge effect
web/src/components/ActionBar/index.tsx            # overlay toggle
```

---

### Task 1: `internal/insights` package skeleton (types, constants, interfaces, sorting)

**Files:**
- Create: `internal/insights/insights.go`
- Test: `internal/insights/insights_test.go`

**Interfaces:**
- Consumes: `internal/model` (`GraphModel`, `TableStats`, `Link`).
- Produces (used by every later task): `Finding`, `CategoryResult`, `Result`, `IndexStat`, `IndexStatser`, `BulkTableStatser`, constants `CategoryIndex/CategoryHealth/CategoryGaps`, `SeverityInfo/SeverityWarn/SeverityCritical`, `StatusOK/StatusUnsupported`, and `sortFindings([]Finding)`.

- [ ] **Step 1: Write the failing test**

`internal/insights/insights_test.go`:

```go
package insights

import (
	"testing"
)

func TestSortFindingsSeverityThenNodeThenTitle(t *testing.T) {
	fs := []Finding{
		{Severity: SeverityInfo, NodeID: "public.b", Title: "z"},
		{Severity: SeverityCritical, NodeID: "public.c", Title: "a"},
		{Severity: SeverityWarn, NodeID: "public.a", Title: "b"},
		{Severity: SeverityWarn, NodeID: "public.a", Title: "a"},
	}
	sortFindings(fs)

	want := []struct{ sev, node, title string }{
		{SeverityCritical, "public.c", "a"},
		{SeverityWarn, "public.a", "a"},
		{SeverityWarn, "public.a", "b"},
		{SeverityInfo, "public.b", "z"},
	}
	for i, w := range want {
		if fs[i].Severity != w.sev || fs[i].NodeID != w.node || fs[i].Title != w.title {
			t.Errorf("pos %d = {%s %s %s}, want {%s %s %s}",
				i, fs[i].Severity, fs[i].NodeID, fs[i].Title, w.sev, w.node, w.title)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/insights/`
Expected: FAIL — `undefined: Finding` (package does not compile yet).

- [ ] **Step 3: Write the implementation**

`internal/insights/insights.go`:

```go
// Package insights computes schema/health findings from a GraphModel and
// optional engine statistics, without touching user row data. Pure functions
// mirror the internal/simulate pattern; engine specifics hide behind narrow
// capability interfaces (pg-insights spec, 2026-07-15).
package insights

import (
	"context"
	"sort"

	"github.com/elgnas/dbviz/internal/model"
)

// Finding categories.
const (
	CategoryIndex  = "index"
	CategoryHealth = "health"
	CategoryGaps   = "gaps"
)

// Finding severities.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)

// Category statuses.
const (
	StatusOK          = "ok"
	StatusUnsupported = "unsupported"
)

// Finding is one actionable observation, keyed to a graph node so the
// frontend can highlight it.
type Finding struct {
	Category string         `json:"category"`
	Severity string         `json:"severity"` // info | warn | critical
	NodeID   string         `json:"nodeId"`
	Title    string         `json:"title"`
	Detail   string         `json:"detail"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// CategoryResult is one category's findings, or its unsupported marker. A
// missing capability degrades to Status "unsupported" inside a 200 payload —
// never an error.
type CategoryResult struct {
	Category string    `json:"category"`
	Status   string    `json:"status"` // ok | unsupported
	Findings []Finding `json:"findings"`
	// ImpliedLinks carries gap candidates as inferred edges, reusing the
	// DESIGN_PLAN §2.3 contract so the frontend dashed renderer works as-is.
	ImpliedLinks []model.Link `json:"impliedLinks,omitempty"`
}

// Result is the full insights response body.
type Result struct {
	Categories []CategoryResult `json:"categories"`
}

// IndexStat is one index's identity and usage statistics, as supplied by an
// IndexStatser capability (Postgres: pg_stat_user_indexes + pg_index).
type IndexStat struct {
	NodeID    string   // "<schema>.<table>"
	Index     string   // index name
	Columns   []string // key columns in index order; expression keys are omitted
	IsUnique  bool
	IsPrimary bool
	IsPartial bool
	Scans     int64 // idx_scan since stats reset
	SizeBytes int64
}

// IndexStatser is an optional adapter capability: bulk index statistics for
// all user tables visible to the connection.
type IndexStatser interface {
	IndexStats(ctx context.Context) ([]IndexStat, error)
}

// BulkTableStatser is an optional adapter capability: table statistics for
// all user tables in one query (bulk variant of Adapter.TableStats).
type BulkTableStatser interface {
	AllTableStats(ctx context.Context) ([]model.TableStats, error)
}

// severityRank orders severities for sorting: critical < warn < info.
func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarn:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}

// sortFindings orders findings by severity, then node, then title, so API
// output is deterministic.
func sortFindings(fs []Finding) {
	sort.Slice(fs, func(i, j int) bool {
		if a, b := severityRank(fs[i].Severity), severityRank(fs[j].Severity); a != b {
			return a < b
		}
		if fs[i].NodeID != fs[j].NodeID {
			return fs[i].NodeID < fs[j].NodeID
		}
		return fs[i].Title < fs[j].Title
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/insights/`
Expected: PASS

- [ ] **Step 5: Vet and commit**

```bash
go vet ./internal/insights/ && gofmt -l internal/insights/
git add internal/insights/insights.go internal/insights/insights_test.go
git commit -m "feat: insights package skeleton — finding types, capability interfaces"
```

---

### Task 2: RelationshipGaps (pure)

**Files:**
- Create: `internal/insights/gaps.go`
- Test: `internal/insights/gaps_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces: `RelationshipGaps(g *model.GraphModel) ([]Finding, []model.Link)` — findings sorted via `sortFindings`; one `model.Link{Inferred: true, Confidence: …}` per finding, in the same order.

Heuristic (spec §Architecture): a column named `<x>_id` on a table-kind node, not a PK, not already an FK, whose type string exactly equals the single-column PK type of a same-schema table labeled `x` (exact match, confidence 0.9, severity warn) or a plural form of `x` (`x+"s"`, `x+"es"`, trailing-`y`→`ies`; confidence 0.75, severity info). One finding per column — highest confidence wins, ties broken by target node ID. Canonical type strings (e.g. `"uuid (uuid)"`) are compared for exact equality.

- [ ] **Step 1: Write the failing tests**

`internal/insights/gaps_test.go`:

```go
package insights

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

const uuidType = "uuid (uuid)"

func gapsGraph() *model.GraphModel {
	return &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.users", Schema: "public", Label: "users", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "email", Type: "string (varchar(255))"},
				}},
			{ID: "public.team", Schema: "public", Label: "team", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
				}},
			{ID: "public.audit_logs", Schema: "public", Label: "audit_logs", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: uuidType},                       // plural match -> users
					{Name: "team_id", Type: uuidType},                       // exact match -> team
					{Name: "session_id", Type: uuidType},                    // no such table
					{Name: "batch_id", Type: "bigint (int8)"},               // type mismatch vs nothing
				}},
			{ID: "public.orders", Schema: "public", Label: "orders", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: uuidType, IsFK: true,
						FKRef: &model.FKRef{Table: "public.users", Column: "id"}}, // real FK -> skip
				}},
			{ID: "public.legacy", Schema: "public", Label: "legacy", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: "bigint (int8)"}, // type mismatch vs users PK -> skip
				}},
		},
	}
}

func TestRelationshipGaps(t *testing.T) {
	findings, links := RelationshipGaps(gapsGraph())

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(findings), findings)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 implied links, got %d: %+v", len(links), links)
	}

	byColumn := map[string]Finding{}
	for _, f := range findings {
		byColumn[f.Meta["column"].(string)] = f
	}

	teamGap, ok := byColumn["team_id"]
	if !ok {
		t.Fatal("expected a gap finding for audit_logs.team_id")
	}
	if teamGap.Severity != SeverityWarn {
		t.Errorf("exact-name match severity = %s, want warn", teamGap.Severity)
	}
	if teamGap.Meta["targetNodeId"] != "public.team" || teamGap.Meta["confidence"] != 0.9 {
		t.Errorf("team_id meta = %+v", teamGap.Meta)
	}
	if teamGap.Category != CategoryGaps || teamGap.NodeID != "public.audit_logs" {
		t.Errorf("team_id finding = %+v", teamGap)
	}

	userGap, ok := byColumn["user_id"]
	if !ok {
		t.Fatal("expected a gap finding for audit_logs.user_id")
	}
	if userGap.Severity != SeverityInfo || userGap.Meta["confidence"] != 0.75 {
		t.Errorf("plural match = %+v", userGap)
	}
	if userGap.Meta["targetNodeId"] != "public.users" {
		t.Errorf("user_id target = %v, want public.users", userGap.Meta["targetNodeId"])
	}

	for _, l := range links {
		if !l.Inferred || l.Confidence == 0 || l.Source != "public.audit_logs" {
			t.Errorf("implied link not inferred/confident/sourced correctly: %+v", l)
		}
		if l.Type != model.Link1ToN {
			t.Errorf("implied link type = %s, want 1:N", l.Type)
		}
	}
}

func TestRelationshipGapsEmptyGraph(t *testing.T) {
	findings, links := RelationshipGaps(&model.GraphModel{})
	if len(findings) != 0 || len(links) != 0 {
		t.Errorf("expected nothing, got %d findings, %d links", len(findings), len(links))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/insights/ -run TestRelationshipGaps -v`
Expected: FAIL — `undefined: RelationshipGaps`.

- [ ] **Step 3: Write the implementation**

`internal/insights/gaps.go`:

```go
package insights

import (
	"fmt"
	"strings"

	"github.com/elgnas/dbviz/internal/model"
)

// Gap confidence scores (spec: exact name+type match scores higher than a
// derived plural match).
const (
	gapConfidenceExact  = 0.9
	gapConfidencePlural = 0.75
)

// pkTarget is a table with a single-column primary key — a candidate target
// for an implied foreign key.
type pkTarget struct {
	nodeID string
	pkType string
}

// RelationshipGaps finds columns that look like foreign keys but have no
// constraint, using catalog heuristics only (no data sampling). It returns
// one Finding per candidate column plus a matching inferred model.Link per
// finding (same order), reusing the §2.3 inferred-edge contract.
func RelationshipGaps(g *model.GraphModel) ([]Finding, []model.Link) {
	// Index candidate targets by "<schema>|<label>".
	targets := map[string]pkTarget{}
	for _, n := range g.Nodes {
		if n.Kind != model.KindTable {
			continue
		}
		var pk *model.Column
		pkCount := 0
		for i := range n.Columns {
			if n.Columns[i].IsPK {
				pk = &n.Columns[i]
				pkCount++
			}
		}
		if pkCount != 1 {
			continue // composite or missing PK — not a v1 target
		}
		targets[n.Schema+"|"+n.Label] = pkTarget{nodeID: n.ID, pkType: pk.Type}
	}

	var findings []Finding
	linkByKey := map[string]model.Link{} // finding key -> link, to re-emit in sorted order
	for _, n := range g.Nodes {
		if n.Kind != model.KindTable {
			continue
		}
		for _, c := range n.Columns {
			if c.IsPK || c.IsFK || !strings.HasSuffix(c.Name, "_id") {
				continue
			}
			base := strings.TrimSuffix(c.Name, "_id")
			if base == "" {
				continue
			}
			target, conf, ok := bestTarget(targets, n.Schema, base, c.Type)
			if !ok || target.nodeID == n.ID {
				continue
			}
			sev := SeverityInfo
			if conf >= gapConfidenceExact {
				sev = SeverityWarn
			}
			f := Finding{
				Category: CategoryGaps,
				Severity: sev,
				NodeID:   n.ID,
				Title:    "Possible missing foreign key",
				Detail: fmt.Sprintf("%s.%s matches %s but has no foreign-key constraint",
					n.ID, c.Name, target.nodeID),
				Meta: map[string]any{
					"column":       c.Name,
					"targetNodeId": target.nodeID,
					"confidence":   conf,
				},
			}
			findings = append(findings, f)
			linkByKey[gapKey(f)] = model.Link{
				Source:     n.ID,
				Target:     target.nodeID,
				Type:       model.Link1ToN,
				ViaColumn:  c.Name,
				Inferred:   true,
				Confidence: conf,
			}
		}
	}

	sortFindings(findings)
	links := make([]model.Link, 0, len(findings))
	for _, f := range findings {
		links = append(links, linkByKey[gapKey(f)])
	}
	return findings, links
}

func gapKey(f Finding) string {
	return f.NodeID + "|" + f.Meta["column"].(string)
}

// bestTarget picks the highest-confidence same-schema target table whose
// single-column PK type exactly equals colType. Exact label match beats
// plural-derived matches; ties break on node ID.
func bestTarget(targets map[string]pkTarget, schema, base, colType string) (pkTarget, float64, bool) {
	type cand struct {
		label string
		conf  float64
	}
	cands := []cand{{base, gapConfidenceExact}}
	for _, p := range pluralForms(base) {
		cands = append(cands, cand{p, gapConfidencePlural})
	}

	var best pkTarget
	bestConf := 0.0
	found := false
	for _, c := range cands {
		t, ok := targets[schema+"|"+c.label]
		if !ok || t.pkType != colType {
			continue
		}
		if !found || c.conf > bestConf || (c.conf == bestConf && t.nodeID < best.nodeID) {
			best, bestConf, found = t, c.conf, true
		}
	}
	return best, bestConf, found
}

// pluralForms derives naive plural table names from a singular base:
// user -> users, box -> boxes, category -> categories.
func pluralForms(base string) []string {
	forms := []string{base + "s", base + "es"}
	if strings.HasSuffix(base, "y") && len(base) > 1 {
		forms = append(forms, base[:len(base)-1]+"ies")
	}
	return forms
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/insights/ -v`
Expected: PASS (all tests so far).

- [ ] **Step 5: Vet and commit**

```bash
go vet ./internal/insights/ && gofmt -l internal/insights/
git add internal/insights/gaps.go internal/insights/gaps_test.go
git commit -m "feat: relationship-gap heuristic — implied FK findings + inferred links"
```

---

### Task 3: IndexFindings (pure)

**Files:**
- Create: `internal/insights/index.go`
- Test: `internal/insights/index_test.go`

**Interfaces:**
- Consumes: Task 1 `IndexStat`, `Finding`; `model.GraphModel`.
- Produces: `IndexFindings(stats []IndexStat, g *model.GraphModel) []Finding`.

Rules:
- Stats for node IDs absent from the graph are ignored.
- **Duplicate:** within a node, indexes with identical column lists (partial indexes and expression-only indexes excluded) group together; the keeper is primary > unique > lexicographically-first name; every other member gets a `warn` "Duplicate index" finding.
- **Redundant:** a non-primary, non-unique, non-partial index whose column list is a strict prefix of another index's on the same node gets an `info` "Redundant index" finding (skipped if already reported duplicate).
- **Unused:** `Scans == 0` and not primary/unique/partial → "Unused index"; `warn` if `SizeBytes >= 10MiB` else `info` (skipped if already reported duplicate or redundant).
- **Missing FK index:** for every graph column with `IsFK`, if no index on that node has that column as its **first** key column → `warn` "Foreign key without index".

- [ ] **Step 1: Write the failing tests**

`internal/insights/index_test.go`:

```go
package insights

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

func indexGraph() *model.GraphModel {
	return &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.users", Schema: "public", Label: "users", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "email", Type: "string (varchar(255))"},
				}},
			{ID: "public.orders", Schema: "public", Label: "orders", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: uuidType, IsFK: true,
						FKRef: &model.FKRef{Table: "public.users", Column: "id"}},
					{Name: "created_at", Type: "timestamp (timestamptz)"},
				}},
		},
	}
}

func titlesFor(fs []Finding, nodeID string) map[string][]Finding {
	out := map[string][]Finding{}
	for _, f := range fs {
		if f.NodeID == nodeID {
			out[f.Title] = append(out[f.Title], f)
		}
	}
	return out
}

func TestIndexFindingsDuplicates(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "public.users", Index: "users_pkey", Columns: []string{"id"}, IsPrimary: true, IsUnique: true, Scans: 10},
		{NodeID: "public.users", Index: "users_email_key", Columns: []string{"email"}, IsUnique: true, Scans: 5},
		{NodeID: "public.users", Index: "idx_users_email", Columns: []string{"email"}, Scans: 0},
		{NodeID: "public.users", Index: "idx_users_email_dup", Columns: []string{"email"}, Scans: 0},
	}
	fs := IndexFindings(stats, indexGraph())
	dups := titlesFor(fs, "public.users")["Duplicate index"]
	if len(dups) != 2 {
		t.Fatalf("expected 2 duplicate findings, got %d: %+v", len(dups), fs)
	}
	for _, d := range dups {
		if d.Severity != SeverityWarn || d.Meta["duplicateOf"] != "users_email_key" {
			t.Errorf("duplicate finding = %+v", d)
		}
	}
	// Duplicates must not also be reported unused.
	if unused := titlesFor(fs, "public.users")["Unused index"]; len(unused) != 0 {
		t.Errorf("duplicate indexes double-reported as unused: %+v", unused)
	}
}

func TestIndexFindingsRedundantPrefix(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "public.orders", Index: "idx_orders_user", Columns: []string{"user_id"}, Scans: 3},
		{NodeID: "public.orders", Index: "idx_orders_user_created", Columns: []string{"user_id", "created_at"}, Scans: 7},
	}
	fs := IndexFindings(stats, indexGraph())
	red := titlesFor(fs, "public.orders")["Redundant index"]
	if len(red) != 1 {
		t.Fatalf("expected 1 redundant finding, got %+v", fs)
	}
	if red[0].Severity != SeverityInfo || red[0].Meta["index"] != "idx_orders_user" ||
		red[0].Meta["coveredBy"] != "idx_orders_user_created" {
		t.Errorf("redundant finding = %+v", red[0])
	}
}

func TestIndexFindingsUnused(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "public.orders", Index: "idx_orders_user", Columns: []string{"user_id"}, Scans: 0, SizeBytes: 20 << 20},
		{NodeID: "public.orders", Index: "idx_orders_created", Columns: []string{"created_at"}, Scans: 0, SizeBytes: 1 << 20},
		{NodeID: "public.orders", Index: "orders_pkey", Columns: []string{"id"}, IsPrimary: true, IsUnique: true, Scans: 0},
	}
	fs := IndexFindings(stats, indexGraph())
	unused := titlesFor(fs, "public.orders")["Unused index"]
	if len(unused) != 2 {
		t.Fatalf("expected 2 unused findings (pkey excluded), got %+v", fs)
	}
	bySev := map[string]int{}
	for _, u := range unused {
		bySev[u.Severity]++
	}
	if bySev[SeverityWarn] != 1 || bySev[SeverityInfo] != 1 {
		t.Errorf("size-based severity split wrong: %+v", unused)
	}
}

func TestIndexFindingsMissingFKIndex(t *testing.T) {
	// No index on orders.user_id at all.
	stats := []IndexStat{
		{NodeID: "public.orders", Index: "orders_pkey", Columns: []string{"id"}, IsPrimary: true, IsUnique: true, Scans: 1},
	}
	fs := IndexFindings(stats, indexGraph())
	missing := titlesFor(fs, "public.orders")["Foreign key without index"]
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing-FK-index finding, got %+v", fs)
	}
	if missing[0].Severity != SeverityWarn || missing[0].Meta["column"] != "user_id" ||
		missing[0].Meta["refTable"] != "public.users" {
		t.Errorf("missing-FK finding = %+v", missing[0])
	}

	// An index whose FIRST column is user_id satisfies the FK.
	stats = append(stats, IndexStat{
		NodeID: "public.orders", Index: "idx_orders_user", Columns: []string{"user_id"}, Scans: 1,
	})
	fs = IndexFindings(stats, indexGraph())
	if missing := titlesFor(fs, "public.orders")["Foreign key without index"]; len(missing) != 0 {
		t.Errorf("indexed FK still reported: %+v", missing)
	}
}

func TestIndexFindingsIgnoresUnknownNodes(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "other.ghost", Index: "idx_ghost", Columns: []string{"x"}, Scans: 0},
	}
	for _, f := range IndexFindings(stats, indexGraph()) {
		if f.NodeID == "other.ghost" {
			t.Errorf("finding emitted for node absent from graph: %+v", f)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/insights/ -run TestIndexFindings -v`
Expected: FAIL — `undefined: IndexFindings`.

- [ ] **Step 3: Write the implementation**

`internal/insights/index.go`:

```go
package insights

import (
	"fmt"
	"sort"
	"strings"

	"github.com/elgnas/dbviz/internal/model"
)

// unusedWarnBytes escalates an unused-index finding from info to warn when the
// wasted space is significant.
const unusedWarnBytes = 10 << 20 // 10 MiB

// IndexFindings analyzes bulk index statistics against the graph: duplicate
// and redundant (prefix) indexes, never-scanned indexes, and FK columns with
// no supporting index. Stats for tables absent from the graph are ignored.
func IndexFindings(stats []IndexStat, g *model.GraphModel) []Finding {
	nodes := map[string]*model.Node{}
	for i := range g.Nodes {
		nodes[g.Nodes[i].ID] = &g.Nodes[i]
	}

	byNode := map[string][]IndexStat{}
	for _, s := range stats {
		if _, ok := nodes[s.NodeID]; ok {
			byNode[s.NodeID] = append(byNode[s.NodeID], s)
		}
	}

	var findings []Finding
	for nodeID, idxs := range byNode {
		reported := map[string]bool{} // index name -> already has a dup/redundant finding

		// --- Duplicates: identical column lists. ---
		groups := map[string][]IndexStat{}
		for _, s := range idxs {
			if s.IsPartial || len(s.Columns) == 0 {
				continue
			}
			key := strings.Join(s.Columns, "\x00")
			groups[key] = append(groups[key], s)
		}
		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			sort.Slice(group, func(i, j int) bool { return keeperRank(group[i]) < keeperRank(group[j]) })
			keeper := group[0]
			for _, dup := range group[1:] {
				if dup.IsPrimary || dup.IsUnique {
					continue // constraint-backed indexes are never droppable duplicates
				}
				reported[dup.Index] = true
				findings = append(findings, Finding{
					Category: CategoryIndex,
					Severity: SeverityWarn,
					NodeID:   nodeID,
					Title:    "Duplicate index",
					Detail: fmt.Sprintf("%s duplicates %s on (%s)",
						dup.Index, keeper.Index, strings.Join(dup.Columns, ", ")),
					Meta: map[string]any{
						"index":       dup.Index,
						"duplicateOf": keeper.Index,
						"columns":     dup.Columns,
						"sizeBytes":   dup.SizeBytes,
					},
				})
			}
		}

		// --- Redundant: strict column-list prefix of a wider index. ---
		for _, a := range idxs {
			if a.IsPrimary || a.IsUnique || a.IsPartial || len(a.Columns) == 0 || reported[a.Index] {
				continue
			}
			for _, b := range idxs {
				if a.Index == b.Index || b.IsPartial || len(b.Columns) <= len(a.Columns) {
					continue
				}
				if isPrefix(a.Columns, b.Columns) {
					reported[a.Index] = true
					findings = append(findings, Finding{
						Category: CategoryIndex,
						Severity: SeverityInfo,
						NodeID:   nodeID,
						Title:    "Redundant index",
						Detail: fmt.Sprintf("%s (%s) is a prefix of %s (%s)",
							a.Index, strings.Join(a.Columns, ", "),
							b.Index, strings.Join(b.Columns, ", ")),
						Meta: map[string]any{
							"index":     a.Index,
							"coveredBy": b.Index,
							"sizeBytes": a.SizeBytes,
						},
					})
					break
				}
			}
		}

		// --- Unused: zero scans since the stats were last reset. ---
		for _, s := range idxs {
			if s.Scans != 0 || s.IsPrimary || s.IsUnique || s.IsPartial || reported[s.Index] {
				continue
			}
			sev := SeverityInfo
			if s.SizeBytes >= unusedWarnBytes {
				sev = SeverityWarn
			}
			findings = append(findings, Finding{
				Category: CategoryIndex,
				Severity: sev,
				NodeID:   nodeID,
				Title:    "Unused index",
				Detail:   fmt.Sprintf("%s has 0 scans since statistics were last reset", s.Index),
				Meta: map[string]any{
					"index":     s.Index,
					"scans":     s.Scans,
					"sizeBytes": s.SizeBytes,
				},
			})
		}
	}

	// --- FK columns with no supporting index (first key column). ---
	for _, n := range g.Nodes {
		firstCols := map[string]bool{}
		for _, s := range byNode[n.ID] {
			if len(s.Columns) > 0 {
				firstCols[s.Columns[0]] = true
			}
		}
		for _, c := range n.Columns {
			if !c.IsFK || firstCols[c.Name] {
				continue
			}
			refTable := ""
			if c.FKRef != nil {
				refTable = c.FKRef.Table
			}
			findings = append(findings, Finding{
				Category: CategoryIndex,
				Severity: SeverityWarn,
				NodeID:   n.ID,
				Title:    "Foreign key without index",
				Detail: fmt.Sprintf("column %s references %s but has no supporting index",
					c.Name, refTable),
				Meta: map[string]any{
					"column":   c.Name,
					"refTable": refTable,
				},
			})
		}
	}

	sortFindings(findings)
	return findings
}

// keeperRank picks which member of a duplicate group survives:
// primary beats unique beats lexicographically-first name.
func keeperRank(s IndexStat) string {
	switch {
	case s.IsPrimary:
		return "0" + s.Index
	case s.IsUnique:
		return "1" + s.Index
	default:
		return "2" + s.Index
	}
}

func isPrefix(short, long []string) bool {
	if len(short) >= len(long) {
		return false
	}
	for i := range short {
		if short[i] != long[i] {
			return false
		}
	}
	return true
}
```

Note: the missing-FK-index loop must consult `byNode` (graph-filtered stats). A node with FK columns but **zero** index stats still yields findings — that is intended (no index exists at all).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/insights/ -v`
Expected: PASS

- [ ] **Step 5: Vet and commit**

```bash
go vet ./internal/insights/ && gofmt -l internal/insights/
git add internal/insights/index.go internal/insights/index_test.go
git commit -m "feat: index findings — duplicate, redundant, unused, unindexed-FK"
```

---

### Task 4: TableStats scan fields + HealthFindings (pure)

**Files:**
- Modify: `internal/model/graph.go:130-141` (TableStats struct)
- Create: `internal/insights/health.go`
- Test: `internal/insights/health_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces: `model.TableStats.SeqScans`/`IdxScans` (`int64`, JSON `seqScans`/`idxScans`, omitempty — `/tables/{nodeId}/stats` response stays backward-compatible); `HealthFindings(stats []model.TableStats) []Finding`. Callers (Task 5's `Collect`) prefilter stats to graph nodes — `HealthFindings` trusts its input.

Thresholds (constants in `health.go`):
- Dead-tuple ratio `dead/(live+dead)`: evaluated only when `live+dead >= 1000`; `>= 0.20` → warn, `>= 0.50` → critical. Meta carries `estBloatBytes = ratio * SizeBytes` (the v1 bloat estimate).
- Never vacuumed: `LastVacuum == "" && LastAutovacuum == ""` with `DeadTuples > 0` and `live+dead >= 1000` → info.
- Seq-scan heavy: `SeqScans >= 100 && RowCount >= 10000 && (IdxScans == 0 || SeqScans >= 10*IdxScans)` → warn.

(A "never analyzed" finding is deliberately out of v1: `model.TableStats` has no `last_autoanalyze` field and the spec only adds `SeqScans`/`IdxScans`.)

- [ ] **Step 1: Add the two fields to model.TableStats**

In `internal/model/graph.go`, inside `TableStats`, after the `Deletes` field:

```go
	SeqScans       int64  `json:"seqScans,omitempty"`
	IdxScans       int64  `json:"idxScans,omitempty"`
```

- [ ] **Step 2: Write the failing tests**

`internal/insights/health_test.go`:

```go
package insights

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

func TestHealthFindingsDeadTupleRatio(t *testing.T) {
	cases := []struct {
		name     string
		live     int64
		dead     int64
		wantSev  string // "" = no dead-ratio finding expected
	}{
		{"critical", 400, 600, SeverityCritical}, // 60% dead
		{"warn", 750, 250, SeverityWarn},         // 25% dead
		{"below threshold", 950, 50, ""},         // 5% dead
		{"too small to matter", 40, 60, ""},      // 60% dead but 100 rows total
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := HealthFindings([]model.TableStats{{
				NodeID: "public.t", RowCount: tc.live, DeadTuples: tc.dead,
				SizeBytes: 1 << 20, LastVacuum: "2026-07-01T00:00:00Z",
			}})
			var got string
			for _, f := range fs {
				if f.Title == "High dead-tuple ratio" {
					got = f.Severity
					if f.Meta["estBloatBytes"] == nil {
						t.Error("dead-ratio finding missing estBloatBytes meta")
					}
				}
			}
			if got != tc.wantSev {
				t.Errorf("severity = %q, want %q (findings: %+v)", got, tc.wantSev, fs)
			}
		})
	}
}

func TestHealthFindingsNeverVacuumed(t *testing.T) {
	fs := HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 5000, DeadTuples: 100,
	}})
	found := false
	for _, f := range fs {
		if f.Title == "Never vacuumed" && f.Severity == SeverityInfo {
			found = true
		}
	}
	if !found {
		t.Errorf("expected never-vacuumed finding, got %+v", fs)
	}

	// Autovacuum counts as vacuumed.
	fs = HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 5000, DeadTuples: 100,
		LastAutovacuum: "2026-07-01T00:00:00Z",
	}})
	for _, f := range fs {
		if f.Title == "Never vacuumed" {
			t.Errorf("autovacuumed table reported never-vacuumed: %+v", f)
		}
	}
}

func TestHealthFindingsSeqScanHeavy(t *testing.T) {
	fs := HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 50_000, SeqScans: 500, IdxScans: 10,
		LastVacuum: "2026-07-01T00:00:00Z",
	}})
	found := false
	for _, f := range fs {
		if f.Title == "Sequential-scan heavy" && f.Severity == SeverityWarn {
			found = true
		}
	}
	if !found {
		t.Errorf("expected seq-scan-heavy finding, got %+v", fs)
	}

	// Healthy index usage -> no finding.
	fs = HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 50_000, SeqScans: 500, IdxScans: 400,
		LastVacuum: "2026-07-01T00:00:00Z",
	}})
	for _, f := range fs {
		if f.Title == "Sequential-scan heavy" {
			t.Errorf("healthy table reported seq-scan heavy: %+v", f)
		}
	}

	// Small tables are exempt.
	fs = HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 500, SeqScans: 5000,
		LastVacuum: "2026-07-01T00:00:00Z",
	}})
	for _, f := range fs {
		if f.Title == "Sequential-scan heavy" {
			t.Errorf("small table reported seq-scan heavy: %+v", f)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/insights/ -run TestHealthFindings -v`
Expected: FAIL — `undefined: HealthFindings`.

- [ ] **Step 4: Write the implementation**

`internal/insights/health.go`:

```go
package insights

import (
	"fmt"

	"github.com/elgnas/dbviz/internal/model"
)

// Table-health thresholds. Small tables are exempt so a handful of dead
// tuples on a tiny table does not spam findings.
const (
	deadRatioWarn     = 0.20
	deadRatioCritical = 0.50
	deadRatioMinRows  = 1000 // live+dead below this -> skip ratio + vacuum checks

	seqScanMinScans    = 100
	seqScanMinRows     = 10_000
	seqScanRatioFactor = 10
)

// HealthFindings analyzes per-table statistics: dead-tuple ratio (with a
// bloat estimate), never-vacuumed tables, and seq-scan-heavy tables. Callers
// prefilter stats to nodes present in the graph.
func HealthFindings(stats []model.TableStats) []Finding {
	var findings []Finding
	for _, s := range stats {
		total := s.RowCount + s.DeadTuples

		if total >= deadRatioMinRows && s.DeadTuples > 0 {
			ratio := float64(s.DeadTuples) / float64(total)
			if ratio >= deadRatioWarn {
				sev := SeverityWarn
				if ratio >= deadRatioCritical {
					sev = SeverityCritical
				}
				findings = append(findings, Finding{
					Category: CategoryHealth,
					Severity: sev,
					NodeID:   s.NodeID,
					Title:    "High dead-tuple ratio",
					Detail: fmt.Sprintf("%.0f%% of tuples are dead (%d of %d)",
						ratio*100, s.DeadTuples, total),
					Meta: map[string]any{
						"deadTuples":    s.DeadTuples,
						"liveTuples":    s.RowCount,
						"ratio":         ratio,
						"estBloatBytes": int64(ratio * float64(s.SizeBytes)),
						"lastVacuum":    s.LastVacuum,
						"lastAutovacuum": s.LastAutovacuum,
					},
				})
			}

			if s.LastVacuum == "" && s.LastAutovacuum == "" {
				findings = append(findings, Finding{
					Category: CategoryHealth,
					Severity: SeverityInfo,
					NodeID:   s.NodeID,
					Title:    "Never vacuumed",
					Detail:   "table has dead tuples but has never been vacuumed (manual or auto)",
					Meta: map[string]any{
						"deadTuples": s.DeadTuples,
					},
				})
			}
		}

		if s.SeqScans >= seqScanMinScans && s.RowCount >= seqScanMinRows &&
			(s.IdxScans == 0 || s.SeqScans >= seqScanRatioFactor*s.IdxScans) {
			findings = append(findings, Finding{
				Category: CategoryHealth,
				Severity: SeverityWarn,
				NodeID:   s.NodeID,
				Title:    "Sequential-scan heavy",
				Detail: fmt.Sprintf("%d sequential scans vs %d index scans",
					s.SeqScans, s.IdxScans),
				Meta: map[string]any{
					"seqScans": s.SeqScans,
					"idxScans": s.IdxScans,
					"rowCount": s.RowCount,
				},
			})
		}
	}
	sortFindings(findings)
	return findings
}
```

- [ ] **Step 5: Run tests + full unit suite**

Run: `go test ./internal/insights/ ./internal/model/ -v` then `go build ./...`
Expected: PASS, clean build (nothing else touches the new fields yet).

- [ ] **Step 6: Vet and commit**

```bash
go vet ./internal/insights/ ./internal/model/ && gofmt -l internal/insights/ internal/model/
git add internal/model/graph.go internal/insights/health.go internal/insights/health_test.go
git commit -m "feat: table-health findings + SeqScans/IdxScans on TableStats"
```

---

### Task 5: Collect orchestrator (capability degradation)

**Files:**
- Create: `internal/insights/collect.go`
- Test: `internal/insights/collect_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–4.
- Produces: `Collect(ctx context.Context, g *model.GraphModel, src any, category string) (Result, error)` — the single entry point the HTTP handler calls. `src` is the adapter; capabilities are type-asserted. `category` `""` means all; invalid values return `*model.APIError` with `ErrBadRequest`. Category order is always index, health, gaps. `Findings` is never nil (serializes as `[]`).

- [ ] **Step 1: Write the failing tests**

`internal/insights/collect_test.go`:

```go
package insights

import (
	"context"
	"errors"
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

// stubStatser implements both capabilities.
type stubStatser struct {
	idx []IndexStat
	tbl []model.TableStats
	err error
}

func (s stubStatser) IndexStats(context.Context) ([]IndexStat, error)          { return s.idx, s.err }
func (s stubStatser) AllTableStats(context.Context) ([]model.TableStats, error) { return s.tbl, s.err }

// bare implements neither capability.
type bare struct{}

func catByName(r Result, name string) *CategoryResult {
	for i := range r.Categories {
		if r.Categories[i].Category == name {
			return &r.Categories[i]
		}
	}
	return nil
}

func TestCollectUnsupportedCapabilities(t *testing.T) {
	r, err := Collect(context.Background(), gapsGraph(), bare{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Categories) != 3 {
		t.Fatalf("expected 3 categories, got %+v", r.Categories)
	}
	if got := catByName(r, CategoryIndex); got == nil || got.Status != StatusUnsupported {
		t.Errorf("index category = %+v, want unsupported", got)
	}
	if got := catByName(r, CategoryHealth); got == nil || got.Status != StatusUnsupported {
		t.Errorf("health category = %+v, want unsupported", got)
	}
	gaps := catByName(r, CategoryGaps)
	if gaps == nil || gaps.Status != StatusOK {
		t.Fatalf("gaps category = %+v, want ok", gaps)
	}
	if len(gaps.Findings) == 0 || len(gaps.ImpliedLinks) != len(gaps.Findings) {
		t.Errorf("gaps findings/links = %d/%d", len(gaps.Findings), len(gaps.ImpliedLinks))
	}
	// Unsupported categories still carry an empty (non-nil) findings array.
	if catByName(r, CategoryIndex).Findings == nil {
		t.Error("unsupported category has nil findings; must serialize as []")
	}
}

func TestCollectWithCapabilities(t *testing.T) {
	src := stubStatser{
		idx: []IndexStat{
			{NodeID: "public.users", Index: "idx_a", Columns: []string{"email"}, Scans: 0},
		},
		tbl: []model.TableStats{
			{NodeID: "public.users", RowCount: 900, DeadTuples: 600},   // in graph -> analyzed
			{NodeID: "other.ghost", RowCount: 900, DeadTuples: 900_000}, // not in graph -> dropped
		},
	}
	r, err := Collect(context.Background(), gapsGraph(), src, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := catByName(r, CategoryIndex); got.Status != StatusOK || len(got.Findings) == 0 {
		t.Errorf("index category = %+v", got)
	}
	health := catByName(r, CategoryHealth)
	if health.Status != StatusOK {
		t.Fatalf("health category = %+v", health)
	}
	for _, f := range health.Findings {
		if f.NodeID == "other.ghost" {
			t.Errorf("stats for node absent from graph were not prefiltered: %+v", f)
		}
	}
}

func TestCollectCategoryFilter(t *testing.T) {
	r, err := Collect(context.Background(), gapsGraph(), bare{}, CategoryGaps)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Categories) != 1 || r.Categories[0].Category != CategoryGaps {
		t.Errorf("filtered result = %+v", r.Categories)
	}
}

func TestCollectInvalidCategory(t *testing.T) {
	_, err := Collect(context.Background(), gapsGraph(), bare{}, "bogus")
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != model.ErrBadRequest {
		t.Errorf("err = %v, want APIError BAD_REQUEST", err)
	}
}

func TestCollectCapabilityErrorPropagates(t *testing.T) {
	boom := errors.New("connection reset")
	_, err := Collect(context.Background(), gapsGraph(), stubStatser{err: boom}, CategoryIndex)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapped %v", err, boom)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/insights/ -run TestCollect -v`
Expected: FAIL — `undefined: Collect`.

- [ ] **Step 3: Write the implementation**

`internal/insights/collect.go`:

```go
package insights

import (
	"context"

	"github.com/elgnas/dbviz/internal/model"
)

// Collect runs the requested insight categories over the graph, sourcing
// engine statistics from src's optional capabilities. category "" means all;
// a capability src does not implement degrades that category to
// status "unsupported" — never an error. Capability query errors propagate.
func Collect(ctx context.Context, g *model.GraphModel, src any, category string) (Result, error) {
	switch category {
	case "", CategoryIndex, CategoryHealth, CategoryGaps:
	default:
		return Result{}, model.NewAPIError(model.ErrBadRequest,
			"category must be one of index, health, gaps")
	}
	want := func(c string) bool { return category == "" || category == c }

	var res Result

	if want(CategoryIndex) {
		cr := CategoryResult{Category: CategoryIndex, Status: StatusUnsupported, Findings: []Finding{}}
		if statser, ok := src.(IndexStatser); ok {
			stats, err := statser.IndexStats(ctx)
			if err != nil {
				return Result{}, err
			}
			cr.Status = StatusOK
			cr.Findings = ensure(IndexFindings(stats, g))
		}
		res.Categories = append(res.Categories, cr)
	}

	if want(CategoryHealth) {
		cr := CategoryResult{Category: CategoryHealth, Status: StatusUnsupported, Findings: []Finding{}}
		if statser, ok := src.(BulkTableStatser); ok {
			stats, err := statser.AllTableStats(ctx)
			if err != nil {
				return Result{}, err
			}
			cr.Status = StatusOK
			cr.Findings = ensure(HealthFindings(filterToGraph(stats, g)))
		}
		res.Categories = append(res.Categories, cr)
	}

	if want(CategoryGaps) {
		findings, links := RelationshipGaps(g)
		res.Categories = append(res.Categories, CategoryResult{
			Category:     CategoryGaps,
			Status:       StatusOK,
			Findings:     ensure(findings),
			ImpliedLinks: links,
		})
	}

	return res, nil
}

// filterToGraph drops stats for tables that are not nodes in the graph
// (e.g. schemas outside the introspected set).
func filterToGraph(stats []model.TableStats, g *model.GraphModel) []model.TableStats {
	known := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		known[n.ID] = true
	}
	out := stats[:0]
	for _, s := range stats {
		if known[s.NodeID] {
			out = append(out, s)
		}
	}
	return out
}

// ensure guarantees a non-nil slice so JSON serializes findings as [].
func ensure(fs []Finding) []Finding {
	if fs == nil {
		return []Finding{}
	}
	return fs
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/insights/ -v`
Expected: PASS (whole package).

- [ ] **Step 5: Vet and commit**

```bash
go vet ./internal/insights/ && gofmt -l internal/insights/
git add internal/insights/collect.go internal/insights/collect_test.go
git commit -m "feat: insights Collect — category orchestration + capability degradation"
```

---

### Task 6: Test fixture — gap candidate + duplicate index

**Files:**
- Modify: `testdata/ecommerce.sql` (before the seed-rows section)
- Modify: `internal/adapter/postgres/adapter_integration_test.go:60,84,120`
- Modify: `internal/server/api_integration_test.go:95`

**Interfaces:**
- Produces: fixture table `public.audit_logs` (UUID PK, FK-less `user_id UUID NOT NULL`) and index `idx_users_email_dup` (exact duplicate of `idx_users_email`), which Tasks 7–8's integration tests assert against. Table count moves 8 → 9.

- [ ] **Step 1: Add the fixture objects**

In `testdata/ecommerce.sql`, after the `payments` block and before the `-- Seed rows` comment, insert:

```sql
-- audit_logs deliberately has NO foreign key on user_id: it exercises the
-- relationship-gaps heuristic (pg-insights spec).
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    action VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Deliberate exact duplicate of idx_users_email: exercises duplicate-index detection.
CREATE INDEX idx_users_email_dup ON users(email);
```

(The `GRANT SELECT ON ALL TABLES` at the bottom of the file already covers the new table because grants run last.)

- [ ] **Step 2: Bump the table-count assertions (RED first)**

Run the integration suites to see the failures the fixture introduces:

```bash
GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go \
  test -count=1 ./internal/adapter/postgres/ ./internal/server/
```

Expected: FAIL on the three count assertions (8 vs 9). Then update:

- `internal/adapter/postgres/adapter_integration_test.go:60` — `MinNodes: 8,` → `MinNodes: 9,`
- `internal/adapter/postgres/adapter_integration_test.go:84` — `assert.Equal(t, 8, g.Stats.NodeCount, "ecommerce fixture has 8 tables")` → `assert.Equal(t, 9, g.Stats.NodeCount, "ecommerce fixture has 9 tables")`
- `internal/adapter/postgres/adapter_integration_test.go:120` — `assert.Equal(t, 8, pkTotal, "all 8 fixture tables have a single-column PK")` → `assert.Equal(t, 9, pkTotal, "all 9 fixture tables have a single-column PK")`
- `internal/server/api_integration_test.go:95` — `assert.Equal(t, 8, g.Stats.NodeCount)` → `assert.Equal(t, 9, g.Stats.NodeCount)`

If any OTHER assertion fails on the new counts, fix it the same way — the fixture gained exactly one table (with a single-column PK, zero FKs, zero extra links) and one duplicate index on `users(email)`.

- [ ] **Step 3: Re-run to verify green**

```bash
GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go \
  test -count=1 ./internal/adapter/postgres/ ./internal/server/
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add testdata/ecommerce.sql internal/adapter/postgres/adapter_integration_test.go internal/server/api_integration_test.go
git commit -m "test: seed insights fixtures — FK-less audit_logs.user_id + duplicate email index"
```

---

### Task 7: Postgres capability implementations

**Files:**
- Modify: `internal/adapter/postgres/stats.go` (shared row scan; per-table query gains seq/idx scans)
- Create: `internal/adapter/postgres/insights.go`
- Test: `internal/adapter/postgres/adapter_integration_test.go` (append new test)

**Interfaces:**
- Consumes: `insights.IndexStat`, `insights.IndexStatser`, `insights.BulkTableStatser`; fixture objects from Task 6.
- Produces: `(*Adapter).IndexStats(ctx) ([]insights.IndexStat, error)` and `(*Adapter).AllTableStats(ctx) ([]model.TableStats, error)`, with compile-time assertions. `TableStats` (per-table) now also populates `SeqScans`/`IdxScans`.

- [ ] **Step 1: Write the failing integration test**

Append to `internal/adapter/postgres/adapter_integration_test.go`:

```go
func TestPostgresInsightsCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	_, readerDSN := startPostgres(t)
	ctx := context.Background()

	a := &Adapter{}
	require.NoError(t, a.Open(ctx, model.ConnectionConfig{Engine: "postgres", DSN: readerDSN}))
	defer a.Close()

	// --- IndexStats ---
	idx, err := a.IndexStats(ctx)
	require.NoError(t, err)
	byName := map[string]insights.IndexStat{}
	for _, s := range idx {
		byName[s.Index] = s
	}

	email, ok := byName["idx_users_email"]
	require.True(t, ok, "idx_users_email present")
	assert.Equal(t, "public.users", email.NodeID)
	assert.Equal(t, []string{"email"}, email.Columns)
	assert.False(t, email.IsUnique)

	dup, ok := byName["idx_users_email_dup"]
	require.True(t, ok, "duplicate index present")
	assert.Equal(t, []string{"email"}, dup.Columns)

	pkey, ok := byName["users_pkey"]
	require.True(t, ok, "users_pkey present")
	assert.True(t, pkey.IsPrimary)
	assert.True(t, pkey.IsUnique)

	// --- AllTableStats ---
	tbl, err := a.AllTableStats(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tbl), 9, "bulk stats cover every fixture table")
	statsByID := map[string]model.TableStats{}
	for _, s := range tbl {
		statsByID[s.NodeID] = s
	}
	users, ok := statsByID["public.users"]
	require.True(t, ok, "public.users in bulk stats")
	assert.Greater(t, users.SizeBytes, int64(0))

	// --- Per-table stats still work and now carry scan counters (>= 0). ---
	one, err := a.TableStats(ctx, "public.users")
	require.NoError(t, err)
	assert.Equal(t, "public.users", one.NodeID)
	assert.GreaterOrEqual(t, one.SeqScans, int64(0))
}
```

Add `"github.com/elgnas/dbviz/internal/insights"` to that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go vet ./internal/adapter/postgres/`
Expected: compile error — `a.IndexStats undefined`.

- [ ] **Step 3: Rework `stats.go` — shared scan + scan counters**

Replace the whole of `internal/adapter/postgres/stats.go` with:

```go
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// Shared column list for pg_stat_user_tables reads; scanned by scanTableStats.
// COALESCE on idx_scan: the view's sum() yields NULL for tables with no indexes.
const tableStatsColumns = `
    s.schemaname, s.relname,
    s.n_live_tup, s.n_dead_tup, s.n_tup_ins, s.n_tup_upd, s.n_tup_del,
    s.seq_scan, COALESCE(s.idx_scan, 0),
    s.last_vacuum, s.last_autovacuum, s.last_analyze,
    pg_total_relation_size(format('%I.%I', s.schemaname, s.relname)::regclass) AS size_bytes`

const queryTableStats = `
SELECT` + tableStatsColumns + `
FROM pg_stat_user_tables s
WHERE s.schemaname = $1 AND s.relname = $2;`

// rowScanner matches both pgx.Row and pgx.Rows.
type rowScanner interface{ Scan(dest ...any) error }

// scanTableStats decodes one tableStatsColumns row into a model.TableStats.
func scanTableStats(row rowScanner) (model.TableStats, error) {
	var (
		schema, table                           string
		live, dead, ins, upd, del, seq, idx     int64
		size                                    int64
		lastVacuum, lastAutovacuum, lastAnalyze *time.Time
	)
	if err := row.Scan(&schema, &table, &live, &dead, &ins, &upd, &del, &seq, &idx,
		&lastVacuum, &lastAutovacuum, &lastAnalyze, &size); err != nil {
		return model.TableStats{}, err
	}
	return model.TableStats{
		NodeID:         schema + "." + table,
		RowCount:       live,
		DeadTuples:     dead,
		SizeBytes:      size,
		Inserts:        ins,
		Updates:        upd,
		Deletes:        del,
		SeqScans:       seq,
		IdxScans:       idx,
		LastVacuum:     fmtTime(lastVacuum),
		LastAutovacuum: fmtTime(lastAutovacuum),
		LastAnalyze:    fmtTime(lastAnalyze),
	}, nil
}

// TableStats returns per-table statistics (§6.2, §10.1).
func (a *Adapter) TableStats(ctx context.Context, nodeID string) (*model.TableStats, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	schema, table, err := splitNodeID(nodeID)
	if err != nil {
		return nil, err
	}

	ts, err := scanTableStats(a.pool.QueryRow(ctx, queryTableStats, schema, table))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, model.NewAPIError(model.ErrSchemaNotFound, "table not found: "+nodeID)
		}
		return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
	}
	return &ts, nil
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
```

- [ ] **Step 4: Create `internal/adapter/postgres/insights.go`**

```go
package postgres

import (
	"context"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/insights"
	"github.com/elgnas/dbviz/internal/model"
)

// Compile-time capability assertions (pg-insights spec).
var (
	_ insights.IndexStatser     = (*Adapter)(nil)
	_ insights.BulkTableStatser = (*Adapter)(nil)
)

// Expression index keys have attnum 0 and no pg_attribute row, so the ARRAY
// subquery silently omits them; IndexFindings treats such indexes by their
// remaining plain columns.
const queryIndexStats = `
SELECT
    s.schemaname, s.relname, s.indexrelname,
    s.idx_scan,
    pg_relation_size(s.indexrelid) AS size_bytes,
    i.indisunique, i.indisprimary,
    (i.indpred IS NOT NULL) AS is_partial,
    ARRAY(
        SELECT a.attname
        FROM unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
        ORDER BY k.ord
    )::text[] AS columns
FROM pg_stat_user_indexes s
JOIN pg_index i ON i.indexrelid = s.indexrelid;`

const queryAllTableStats = `
SELECT` + tableStatsColumns + `
FROM pg_stat_user_tables s
ORDER BY s.schemaname, s.relname;`

// IndexStats implements insights.IndexStatser: bulk index usage statistics
// from pg_stat_user_indexes + pg_index (catalog metadata only).
func (a *Adapter) IndexStats(ctx context.Context) ([]insights.IndexStat, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	rows, err := a.pool.Query(ctx, queryIndexStats)
	if err != nil {
		return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
	}
	defer rows.Close()

	var out []insights.IndexStat
	for rows.Next() {
		var (
			schema, table, index string
			s                    insights.IndexStat
		)
		if err := rows.Scan(&schema, &table, &index, &s.Scans, &s.SizeBytes,
			&s.IsUnique, &s.IsPrimary, &s.IsPartial, &s.Columns); err != nil {
			return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
		}
		s.NodeID = schema + "." + table
		s.Index = index
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
	}
	return out, nil
}

// AllTableStats implements insights.BulkTableStatser: one-query bulk variant
// of TableStats over pg_stat_user_tables.
func (a *Adapter) AllTableStats(ctx context.Context) ([]model.TableStats, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	rows, err := a.pool.Query(ctx, queryAllTableStats)
	if err != nil {
		return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
	}
	defer rows.Close()

	var out []model.TableStats
	for rows.Next() {
		ts, err := scanTableStats(rows)
		if err != nil {
			return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
		}
		out = append(out, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
	}
	return out, nil
}
```

- [ ] **Step 5: Run the integration test**

```bash
go vet ./internal/adapter/postgres/ && gofmt -l internal/adapter/postgres/
GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go \
  test -count=1 -run 'TestPostgresInsightsCapabilities|TestPostgresConformance|TestPostgresIntrospectShape' ./internal/adapter/postgres/ -v
```

Expected: PASS. If the `ARRAY(...)::text[]` scan into `&s.Columns` errors, scan into an intermediate `[]string` variable — pgx v5 decodes `text[]` into `[]string` natively.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/postgres/stats.go internal/adapter/postgres/insights.go internal/adapter/postgres/adapter_integration_test.go
git commit -m "feat: postgres insights capabilities — bulk index + table statistics"
```

---

### Task 8: `GET /api/connections/{id}/insights` endpoint

**Files:**
- Modify: `internal/audit/log.go:17-22` (add `OpInsights`)
- Modify: `internal/server/handlers.go` (add `getInsights`)
- Modify: `internal/server/server.go:126-140` (route)
- Test: `internal/server/api_integration_test.go` (append new test)

**Interfaces:**
- Consumes: `insights.Collect` (Task 5), fixture objects (Task 6), capabilities (Task 7).
- Produces: `GET /api/connections/{id}/insights?category=index|health|gaps`, wrapped in `timeout(introspectTimeout)`, returning `insights.Result` as 200 JSON; errors via `writeError`. Audit entries: one `OpIntrospect` ("introspect for insights") + one `OpInsights` for the capability queries.

- [ ] **Step 1: Write the failing integration test**

Append to `internal/server/api_integration_test.go`:

```go
func TestInsightsEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	readerDSN := startPostgresReaderDSN(t)

	router := NewRouter(Config{Dev: true}, nil, connection.NewManager(connection.Config{}, nil), audit.Nop{})
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Create a connection.
	body, _ := json.Marshal(model.ConnectionConfig{DSN: readerDSN})
	res, err := http.Post(srv.URL+"/api/connections", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	var meta connMeta
	require.NoError(t, json.NewDecoder(res.Body).Decode(&meta))
	res.Body.Close()

	// All categories.
	res, err = http.Get(srv.URL + "/api/connections/" + meta.ID + "/insights")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var result insights.Result
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	require.Len(t, result.Categories, 3)

	byCat := map[string]insights.CategoryResult{}
	for _, c := range result.Categories {
		byCat[c.Category] = c
		assert.Equal(t, insights.StatusOK, c.Status, "postgres supports all categories")
		assert.NotNil(t, c.Findings)
	}

	// Gaps: audit_logs.user_id -> public.users, with a matching implied link.
	var gapFound bool
	for _, f := range byCat[insights.CategoryGaps].Findings {
		if f.NodeID == "public.audit_logs" && f.Meta["column"] == "user_id" {
			gapFound = true
			assert.Equal(t, "public.users", f.Meta["targetNodeId"])
		}
	}
	assert.True(t, gapFound, "expected gap finding for audit_logs.user_id")
	assert.NotEmpty(t, byCat[insights.CategoryGaps].ImpliedLinks)

	// Index: the seeded duplicate index is detected.
	var dupFound bool
	for _, f := range byCat[insights.CategoryIndex].Findings {
		if f.Title == "Duplicate index" && f.NodeID == "public.users" {
			dupFound = true
		}
	}
	assert.True(t, dupFound, "expected duplicate-index finding for users(email)")

	// Category filter.
	res2, err := http.Get(srv.URL + "/api/connections/" + meta.ID + "/insights?category=gaps")
	require.NoError(t, err)
	defer res2.Body.Close()
	var filtered insights.Result
	require.NoError(t, json.NewDecoder(res2.Body).Decode(&filtered))
	require.Len(t, filtered.Categories, 1)
	assert.Equal(t, insights.CategoryGaps, filtered.Categories[0].Category)

	// Invalid category -> 400 BAD_REQUEST.
	res3, err := http.Get(srv.URL + "/api/connections/" + meta.ID + "/insights?category=bogus")
	require.NoError(t, err)
	defer res3.Body.Close()
	assert.Equal(t, http.StatusBadRequest, res3.StatusCode)

	// Unknown connection -> 404.
	res4, err := http.Get(srv.URL + "/api/connections/nope/insights")
	require.NoError(t, err)
	defer res4.Body.Close()
	assert.Equal(t, http.StatusNotFound, res4.StatusCode)
}
```

Add `"github.com/elgnas/dbviz/internal/insights"` to that file's imports. (Unsupported-capability degradation is already unit-tested at the `Collect` level in Task 5 — no fake engine needed here.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go vet ./internal/server/`
Expected: compile OK but test would 404 — confirm quickly:

```bash
GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go \
  test -count=1 -run TestInsightsEndpoint ./internal/server/ -v
```

Expected: FAIL — 404 on `/insights` (route absent).

- [ ] **Step 3: Add the audit op**

In `internal/audit/log.go`, extend the operations const block:

```go
const (
	OpIntrospect = "introspect"
	OpExplain    = "explain"
	OpSample     = "sample"
	OpStats      = "stats"
	OpInsights   = "insights"
)
```

- [ ] **Step 4: Add the handler**

In `internal/server/handlers.go`, add `"github.com/elgnas/dbviz/internal/insights"` to imports, then append after `tableStats`:

```go
func (a *api) getInsights(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start := time.Now()
	g, err := mc.Adapter.Introspect(r.Context(), adapter.IntrospectOptions{
		MaxNodes:  maxNodesFromEnv(),
		SkipStats: true,
	})
	nodeCount := 0
	if g != nil {
		nodeCount = g.Stats.NodeCount
	}
	a.rec(mc, audit.OpIntrospect, "introspect for insights", start, nodeCount, err)
	if err != nil {
		writeError(w, err)
		return
	}

	category := r.URL.Query().Get("category")
	start = time.Now()
	res, err := insights.Collect(r.Context(), g, mc.Adapter, category)
	a.rec(mc, audit.OpInsights, "pg_stat_user_indexes + pg_stat_user_tables (bulk)", start, 0, err)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
```

- [ ] **Step 5: Mount the route**

In `internal/server/server.go`, inside the `r.Route("/{id}", …)` block, after the `/tables/{nodeId}/stats` line:

```go
					r.With(timeout(introspectTimeout)).Get("/insights", a.getInsights)
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go vet ./... && gofmt -l internal/
GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go \
  test -count=1 ./internal/server/ ./internal/insights/
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/audit/log.go internal/server/handlers.go internal/server/server.go internal/server/api_integration_test.go
git commit -m "feat: GET /api/connections/{id}/insights endpoint with category filter"
```

---

### Task 9: Frontend data layer — types, API client, store, pure helpers

**Files:**
- Modify: `web/src/types/graph.ts`
- Create: `web/src/api/insights.ts`
- Create: `web/src/store/insights.ts`
- Create: `web/src/lib/insights.ts`
- Test: `web/src/lib/insights.test.ts`

**Interfaces:**
- Consumes: Go JSON shapes from Tasks 1/5 (must mirror exactly).
- Produces: types `Finding`, `InsightCategoryResult`, `InsightsResult`, `InsightSeverity`; `getInsights(connId, category?)`; `useInsightsStore` (`result`, `overlay`, `setResult`, `setOverlay`, `clear`); helpers `severityRank`, `sortFindings`, `impliedLinks`, `mergeImpliedLinks`, `severityByNode`.

- [ ] **Step 1: Write the failing vitest**

`web/src/lib/insights.test.ts`:

```ts
import { describe, expect, it } from 'vitest';

import {
  impliedLinks,
  mergeImpliedLinks,
  severityByNode,
  severityRank,
  sortFindings,
} from './insights';
import type { Finding, GraphModel, InsightsResult, Link } from '@/types/graph';

const finding = (over: Partial<Finding>): Finding => ({
  category: 'gaps',
  severity: 'info',
  nodeId: 'public.a',
  title: 't',
  detail: 'd',
  ...over,
});

const link = (over: Partial<Link>): Link => ({
  source: 'public.a',
  target: 'public.b',
  type: '1:N',
  cascade: false,
  viaColumn: 'b_id',
  inferred: true,
  confidence: 0.75,
  ...over,
});

const result: InsightsResult = {
  categories: [
    {
      category: 'index',
      status: 'ok',
      findings: [finding({ category: 'index', severity: 'warn', nodeId: 'public.b' })],
    },
    {
      category: 'health',
      status: 'unsupported',
      findings: [finding({ category: 'health', severity: 'critical', nodeId: 'public.c' })],
    },
    {
      category: 'gaps',
      status: 'ok',
      findings: [finding({ severity: 'critical', nodeId: 'public.b' })],
      impliedLinks: [link({})],
    },
  ],
};

describe('severityRank / sortFindings', () => {
  it('orders critical < warn < info', () => {
    expect(severityRank('critical')).toBeLessThan(severityRank('warn'));
    expect(severityRank('warn')).toBeLessThan(severityRank('info'));
  });

  it('sorts by severity then node, or node then severity', () => {
    const fs = [
      finding({ severity: 'info', nodeId: 'public.a' }),
      finding({ severity: 'critical', nodeId: 'public.z' }),
    ];
    expect(sortFindings(fs, 'severity')[0].severity).toBe('critical');
    expect(sortFindings(fs, 'table')[0].nodeId).toBe('public.a');
  });
});

describe('impliedLinks', () => {
  it('extracts the gaps category links', () => {
    expect(impliedLinks(result)).toHaveLength(1);
    expect(impliedLinks(null)).toHaveLength(0);
  });
});

describe('mergeImpliedLinks', () => {
  const graph: GraphModel = {
    engine: 'postgres',
    database: 'db',
    schemas: ['public'],
    nodes: [
      { id: 'public.a', schema: 'public', label: 'a', kind: 'table', columns: [], rowCount: 0, sizeBytes: 0 },
      { id: 'public.b', schema: 'public', label: 'b', kind: 'table', columns: [], rowCount: 0, sizeBytes: 0 },
    ],
    links: [link({ inferred: false })],
    stats: { nodeCount: 2, linkCount: 1, cascadeChainDepth: 0, columnCount: 0 },
  };

  it('appends implied links with known endpoints', () => {
    const merged = mergeImpliedLinks(graph, [link({ viaColumn: 'other_id' })]);
    expect(merged.links).toHaveLength(2);
    expect(graph.links).toHaveLength(1); // input untouched
  });

  it('drops duplicates of real links and unknown endpoints', () => {
    const merged = mergeImpliedLinks(graph, [
      link({}), // same source/target/viaColumn as the real link -> dropped
      link({ target: 'public.ghost' }), // unknown endpoint -> dropped
    ]);
    expect(merged.links).toHaveLength(1);
  });
});

describe('severityByNode', () => {
  it('keeps the highest severity per node and skips unsupported categories', () => {
    const map = severityByNode(result);
    expect(map.get('public.b')).toBe('critical'); // gaps critical beats index warn
    expect(map.has('public.c')).toBe(false); // unsupported category ignored
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm --prefix web run test`
Expected: FAIL — cannot resolve `./insights` (and the new types).

- [ ] **Step 3: Add the types**

In `web/src/types/graph.ts`:

Inside `TableStats`, after `deletes?: number;`:

```ts
  seqScans?: number;
  idxScans?: number;
```

At the end of the file (before the `GraphAnimator` comment block):

```ts
// --- Insights (pg-insights spec, phase 1). Mirrors internal/insights. ---

export type InsightCategory = 'index' | 'health' | 'gaps';
export type InsightSeverity = 'info' | 'warn' | 'critical';

export interface Finding {
  category: string;
  severity: InsightSeverity;
  nodeId: string;
  title: string;
  detail: string;
  meta?: Record<string, unknown>;
}

export interface InsightCategoryResult {
  category: InsightCategory;
  status: 'ok' | 'unsupported';
  findings: Finding[];
  impliedLinks?: Link[];
}

export interface InsightsResult {
  categories: InsightCategoryResult[];
}
```

- [ ] **Step 4: Create the API wrapper**

`web/src/api/insights.ts`:

```ts
// Typed wrapper for the insights endpoint (pg-insights spec, phase 1).
import { api } from './client';
import type { InsightsResult } from '@/types/graph';

// GET /api/connections/:id/insights(?category=index|health|gaps) -> InsightsResult
export function getInsights(connId: string, category?: string): Promise<InsightsResult> {
  const qs = category ? `?category=${encodeURIComponent(category)}` : '';
  return api<InsightsResult>(`/api/connections/${encodeURIComponent(connId)}/insights${qs}`);
}
```

- [ ] **Step 5: Create the store**

`web/src/store/insights.ts`:

```ts
// Holds the latest insights result + the graph-overlay toggle. The panel
// writes the result; the ActionBar toggles the overlay; GraphCanvas and the
// Workspace read both to render badges and implied edges.
import { create } from 'zustand';
import type { InsightsResult } from '@/types/graph';

interface InsightsState {
  result: InsightsResult | null;
  overlay: boolean;
  setResult: (result: InsightsResult | null) => void;
  setOverlay: (overlay: boolean) => void;
  clear: () => void;
}

export const useInsightsStore = create<InsightsState>((set) => ({
  result: null,
  overlay: false,
  setResult: (result) => set({ result }),
  setOverlay: (overlay) => set({ overlay }),
  clear: () => set({ result: null, overlay: false }),
}));
```

- [ ] **Step 6: Create the pure helpers**

`web/src/lib/insights.ts`:

```ts
// Pure helpers shared by the InsightsPanel and the graph overlay.
import type { Finding, GraphModel, InsightSeverity, InsightsResult, Link } from '@/types/graph';

const SEVERITY_RANK: Record<string, number> = { critical: 0, warn: 1, info: 2 };

/** Lower rank = more severe. Unknown severities sort last. */
export function severityRank(severity: string): number {
  return SEVERITY_RANK[severity] ?? 3;
}

/** Returns a new array sorted by severity-then-table or table-then-severity. */
export function sortFindings(findings: Finding[], by: 'severity' | 'table'): Finding[] {
  return [...findings].sort((a, b) => {
    if (by === 'table') {
      return a.nodeId.localeCompare(b.nodeId) || severityRank(a.severity) - severityRank(b.severity);
    }
    return severityRank(a.severity) - severityRank(b.severity) || a.nodeId.localeCompare(b.nodeId);
  });
}

/** Extracts the gap candidates' inferred edges from a result. */
export function impliedLinks(result: InsightsResult | null): Link[] {
  return result?.categories.find((c) => c.category === 'gaps')?.impliedLinks ?? [];
}

const linkKey = (l: Link) => `${l.source}->${l.target}:${l.viaColumn}`;

/**
 * mergeImpliedLinks returns a copy of the graph with implied edges appended.
 * Links whose endpoints are missing from the graph, or that duplicate a real
 * link, are dropped. Returns the input graph unchanged when nothing merges.
 */
export function mergeImpliedLinks(graph: GraphModel, links: Link[]): GraphModel {
  if (links.length === 0) return graph;
  const nodeIds = new Set(graph.nodes.map((n) => n.id));
  const real = new Set(graph.links.map(linkKey));
  const extra = links.filter(
    (l) => nodeIds.has(l.source) && nodeIds.has(l.target) && !real.has(linkKey(l)),
  );
  if (extra.length === 0) return graph;
  return { ...graph, links: [...graph.links, ...extra] };
}

/** Highest severity per node across all supported categories. */
export function severityByNode(result: InsightsResult | null): Map<string, InsightSeverity> {
  const map = new Map<string, InsightSeverity>();
  if (!result) return map;
  for (const cat of result.categories) {
    if (cat.status !== 'ok') continue;
    for (const f of cat.findings) {
      const current = map.get(f.nodeId);
      if (!current || severityRank(f.severity) < severityRank(current)) {
        map.set(f.nodeId, f.severity);
      }
    }
  }
  return map;
}
```

- [ ] **Step 7: Run tests + typecheck**

Run: `npm --prefix web run test && npm --prefix web run build`
Expected: vitest PASS, tsc + vite build clean. (If vitest cannot resolve the `@/` alias, it reads `vite.config.ts` automatically — check you ran it via the npm script from the repo root.)

- [ ] **Step 8: Commit**

```bash
git add web/src/types/graph.ts web/src/api/insights.ts web/src/store/insights.ts web/src/lib/insights.ts web/src/lib/insights.test.ts
git commit -m "feat: frontend insights data layer — types, api, store, pure helpers"
```

---

### Task 10: InsightsPanel + aside tabs

**Files:**
- Create: `web/src/components/InsightsPanel/index.tsx`
- Modify: `web/src/App.tsx` (Workspace: aside tabs, insights-store cleanup)

**Interfaces:**
- Consumes: Task 9 (`getInsights`, `useInsightsStore`, `sortFindings`, `severityRank`); `useSelectionStore.select`; `useGraphStore` animator (`pulse`).
- Produces: `<InsightsPanel connId graph />`; Workspace renders it in the right aside behind an Inspector/Insights tab switch. Fetch happens when the panel mounts (i.e. when the user opens the tab) — satisfying "on demand only" — with a manual Refresh button.

- [ ] **Step 1: Create the panel**

`web/src/components/InsightsPanel/index.tsx`:

```tsx
// Insights side panel (pg-insights spec, phase 1). Mounting the panel (the
// user opening the tab) triggers the fetch — insights are on-demand only.
// The result is pushed into the insights store so the graph overlay
// (badges + implied edges) can read it.
import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ArrowUpDown, RefreshCw } from 'lucide-react';

import { getInsights } from '@/api/insights';
import { useGraphStore } from '@/store/graph';
import { useInsightsStore } from '@/store/insights';
import { useSelectionStore } from '@/store/selection';
import { sortFindings } from '@/lib/insights';
import { severityColor } from '@/lib/colors';
import type { Finding, GraphModel, InsightCategoryResult } from '@/types/graph';

const CATEGORY_LABELS: Record<string, string> = {
  index: 'Indexes',
  health: 'Health',
  gaps: 'Gaps',
};

export function InsightsPanel({ connId, graph }: { connId: string; graph: GraphModel }) {
  const [category, setCategory] = useState<string | null>(null);
  const [sortBy, setSortBy] = useState<'severity' | 'table'>('severity');
  const setResult = useInsightsStore((s) => s.setResult);

  const query = useQuery({
    queryKey: ['insights', connId],
    queryFn: () => getInsights(connId),
    staleTime: Infinity,
  });

  // Publish for the graph overlay.
  useEffect(() => {
    setResult(query.data ?? null);
  }, [query.data, setResult]);

  // Unsupported categories are hidden (spec: frontend hides that category).
  const visible = useMemo(
    () => (query.data?.categories ?? []).filter((c) => c.status === 'ok'),
    [query.data],
  );
  const active: InsightCategoryResult | undefined =
    visible.find((c) => c.category === category) ?? visible[0];

  const labelById = useMemo(
    () => new Map(graph.nodes.map((n) => [n.id, n.label])),
    [graph.nodes],
  );

  return (
    <div className="flex h-full flex-col text-[13px]">
      {/* Header: title + sort + refresh */}
      <div className="flex shrink-0 items-center gap-2 border-b border-black/10 px-3 py-2 dark:border-white/10">
        <span className="font-semibold">Insights</span>
        <button
          type="button"
          onClick={() => setSortBy(sortBy === 'severity' ? 'table' : 'severity')}
          title={`Sorted by ${sortBy} — click to toggle`}
          className="ml-auto inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5"
        >
          <ArrowUpDown size={11} />
          {sortBy}
        </button>
        <button
          type="button"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
          title="Re-run insights"
          className="rounded p-1 hover:bg-black/5 disabled:opacity-40 dark:hover:bg-white/5"
        >
          <RefreshCw size={13} className={query.isFetching ? 'animate-spin' : ''} />
        </button>
      </div>

      {/* Category tabs */}
      {visible.length > 0 && (
        <div className="flex shrink-0 gap-1 border-b border-black/10 px-2 py-1.5 dark:border-white/10">
          {visible.map((c) => (
            <button
              key={c.category}
              type="button"
              onClick={() => setCategory(c.category)}
              className={
                'rounded px-2 py-0.5 text-[11px] font-medium ' +
                (active?.category === c.category
                  ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
                  : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
              }
            >
              {CATEGORY_LABELS[c.category] ?? c.category} ({c.findings.length})
            </button>
          ))}
        </div>
      )}

      {/* Body */}
      <div className="min-h-0 flex-1 overflow-auto">
        {query.isLoading && (
          <p className="px-3 py-4 text-neutral-500">Analyzing schema…</p>
        )}
        {query.isError && (
          <p className="px-3 py-4 text-red-600">Failed to load insights.</p>
        )}
        {query.data && active && active.findings.length === 0 && (
          <p className="px-3 py-4 text-neutral-500">
            No {CATEGORY_LABELS[active.category]?.toLowerCase() ?? active.category} findings. 🎉
          </p>
        )}
        {active &&
          sortFindings(active.findings, sortBy).map((f, i) => (
            <FindingRow
              key={`${f.nodeId}-${f.title}-${i}`}
              finding={f}
              label={labelById.get(f.nodeId) ?? f.nodeId}
            />
          ))}
      </div>
    </div>
  );
}

function FindingRow({ finding, label }: { finding: Finding; label: string }) {
  const select = useSelectionStore((s) => s.select);
  const animator = useGraphStore((s) => s.animator);

  return (
    <button
      type="button"
      onClick={() => {
        select(finding.nodeId);
        animator?.pulse(finding.nodeId);
      }}
      title="Highlight this table on the graph"
      className="block w-full border-b border-black/5 px-3 py-2 text-left hover:bg-black/5 dark:border-white/5 dark:hover:bg-white/5"
    >
      <span className="flex items-center gap-1.5">
        <span
          aria-label={finding.severity}
          className="inline-block h-2 w-2 shrink-0 rounded-full"
          style={{ backgroundColor: severityColor(finding.severity) }}
        />
        <span className="font-medium">{finding.title}</span>
        <span className="ml-auto shrink-0 text-[11px] text-neutral-500">{label}</span>
      </span>
      <span className="mt-0.5 block text-[12px] text-neutral-600 dark:text-neutral-400">
        {finding.detail}
      </span>
    </button>
  );
}
```

Note: this imports `severityColor` from `@/lib/colors`, which Task 11 adds. To keep this task independently compilable, add `severityColor` to `web/src/lib/colors.ts` **now** (it is a leaf helper):

```ts
/** Maps an insight severity to its badge color (muted accents, no neon). */
export function severityColor(severity: 'info' | 'warn' | 'critical'): string {
  switch (severity) {
    case 'critical':
      return palette.accentDelete;
    case 'warn':
      return palette.accentUpdate;
    default:
      return palette.accentQuery;
  }
}
```

- [ ] **Step 2: Wire the aside tabs in `web/src/App.tsx`**

Add imports:

```tsx
import { InsightsPanel } from '@/components/InsightsPanel';
import { useInsightsStore } from '@/store/insights';
```

In `Workspace`, add tab state and a per-connection reset next to the existing `useState`:

```tsx
  const [sideTab, setSideTab] = useState<'inspector' | 'insights'>('inspector');

  // Insights are per-connection: drop stale results + overlay when it changes.
  useEffect(() => {
    useInsightsStore.getState().clear();
    setSideTab('inspector');
  }, [conn.id]);
```

(Add `useEffect` to the React import in App.tsx.)

Replace the existing `<aside>…</aside>` block with:

```tsx
        <aside className="flex w-80 shrink-0 flex-col overflow-hidden border-l border-black/10 dark:border-white/10">
          <div className="flex shrink-0 gap-1 border-b border-black/10 px-2 py-1.5 dark:border-white/10">
            <SideTabButton
              active={sideTab === 'inspector'}
              onClick={() => setSideTab('inspector')}
              label="Inspector"
            />
            <SideTabButton
              active={sideTab === 'insights'}
              onClick={() => setSideTab('insights')}
              label="Insights"
            />
          </div>
          <div className="min-h-0 flex-1 overflow-auto">
            {graphQuery.data &&
              (sideTab === 'inspector' ? (
                <TableInspector connId={conn.id} graph={graphQuery.data} />
              ) : (
                <InsightsPanel connId={conn.id} graph={graphQuery.data} />
              ))}
          </div>
        </aside>
```

And add the small tab button component at file scope (near `WarningBanner`):

```tsx
function SideTabButton({
  active,
  onClick,
  label,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        'rounded px-2.5 py-0.5 text-xs font-medium transition-colors ' +
        (active
          ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
          : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
      }
    >
      {label}
    </button>
  );
}
```

- [ ] **Step 3: Typecheck + lint + verify in browser**

Run: `npm --prefix web run build && npm --prefix web run lint`
Expected: clean.

Then verify visually (REQUIRED — use the superpowers:verifying-frontend-changes skill at execution time): start a seeded Postgres, run the backend (`scripts/dev.sh` or `go run . serve --dev`) + `npm --prefix web run dev`, connect, open the Insights tab, and confirm: findings render, category tabs show counts, unsupported categories hidden, clicking a finding selects + pulses the node.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/InsightsPanel/index.tsx web/src/App.tsx web/src/lib/colors.ts
git commit -m "feat: InsightsPanel side tab — sortable findings, click-to-highlight"
```

---

### Task 11: Graph overlay — severity badges, implied edges, ActionBar toggle

**Files:**
- Modify: `web/src/components/Graph/nodeRenderer.ts` (add `renderSeverityBadges`)
- Modify: `web/src/components/Graph/GraphCanvas.tsx` (badge effect)
- Modify: `web/src/App.tsx` (merged display graph)
- Modify: `web/src/components/ActionBar/index.tsx` (overlay toggle)

**Interfaces:**
- Consumes: Task 9 (`useInsightsStore`, `mergeImpliedLinks`, `impliedLinks`, `severityByNode`), Task 10's `severityColor`.
- Produces: severity badge dots on affected nodes; dashed implied-FK edges (free via the existing inferred-link renderer once links are merged); an "Insights" toggle in the ActionBar.

- [ ] **Step 1: Add `renderSeverityBadges` to `nodeRenderer.ts`**

Append to `web/src/components/Graph/nodeRenderer.ts` (before the re-export line), and add the import `import { nodeFill, palette, severityColor } from '@/lib/colors';` (extend the existing import):

```ts
/**
 * renderSeverityBadges draws (or removes) a small severity dot at the
 * top-left of each node circle — the insights overlay. Badges live inside
 * the node <g>, so they track the node on every tick for free. Idempotent:
 * calling with an empty map removes all badges.
 */
export function renderSeverityBadges(
  refs: SimRefs,
  severityByNode: Map<string, 'info' | 'warn' | 'critical'>,
): void {
  refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .each(function (d) {
      const group = select(this);
      const severity = severityByNode.get(d.id);
      const existing = group.select<SVGCircleElement>('circle.dbviz-node-sev');
      if (!severity) {
        existing.remove();
        return;
      }
      const badge = existing.empty()
        ? group.append('circle').attr('class', 'dbviz-node-sev')
        : existing;
      badge
        .attr('r', 3.4)
        .attr('cx', (-d.radius) * 0.7)
        .attr('cy', (-d.radius) * 0.7)
        .attr('fill', severityColor(severity))
        .attr('stroke', 'none')
        .attr('opacity', 0.95);
    });
}
```

- [ ] **Step 2: Apply badges from `GraphCanvas.tsx`**

Add imports:

```tsx
import { useInsightsStore } from '@/store/insights';
import { severityByNode } from '@/lib/insights';
import { renderSeverityBadges } from './nodeRenderer';
```

Inside the `GraphCanvas` component, after the hover-fade effect, add:

```tsx
  // --- Insights overlay: severity badges (imperative, tracks the store). ---
  const insightsResult = useInsightsStore((s) => s.result);
  const overlay = useInsightsStore((s) => s.overlay);
  useEffect(() => {
    const refs = simRef.current;
    if (!refs) return;
    const map = overlay ? severityByNode(insightsResult) : new Map();
    renderSeverityBadges(refs, map);
    // Re-run on soft updates too, so freshly-entered nodes get badges.
  }, [insightsResult, overlay, graph.nodes.length, graph.links.length]);
```

- [ ] **Step 3: Merge implied edges in `Workspace` (App.tsx)**

Add imports:

```tsx
import { useMemo } from 'react'; // extend the existing react import
import { impliedLinks, mergeImpliedLinks } from '@/lib/insights';
```

In `Workspace`, read the store and derive the display graph:

```tsx
  const insightsResult = useInsightsStore((s) => s.result);
  const overlay = useInsightsStore((s) => s.overlay);

  // With the overlay on, gap candidates are appended as inferred links — the
  // existing dashed renderer picks them up via link.inferred (§2.3 contract).
  const displayGraph = useMemo(() => {
    if (!graphQuery.data) return undefined;
    if (!overlay) return graphQuery.data;
    return mergeImpliedLinks(graphQuery.data, impliedLinks(insightsResult));
  }, [graphQuery.data, overlay, insightsResult]);
```

Change the canvas render line from `{graphQuery.data && <GraphCanvas graph={graphQuery.data} />}` to:

```tsx
          {displayGraph && <GraphCanvas graph={displayGraph} />}
```

(TableInspector, InsightsPanel, and ActionBar keep receiving `graphQuery.data` — only the canvas sees merged links.)

- [ ] **Step 4: Add the ActionBar toggle**

In `web/src/components/ActionBar/index.tsx`:

- Add `Lightbulb` to the lucide import (confirm it exists in the installed 1.x `lucide-react`; use `Eye` if not).
- Add `import { useInsightsStore } from '@/store/insights';`
- Inside `ActionBar`, next to the other store reads:

```tsx
  const overlay = useInsightsStore((s) => s.overlay);
  const setOverlay = useInsightsStore((s) => s.setOverlay);
  const hasInsights = useInsightsStore((s) => s.result !== null);
```

- In the tabs row, after the two `TabButton`s and before the `ml-auto` div:

```tsx
        <button
          type="button"
          disabled={!hasInsights}
          onClick={() => setOverlay(!overlay)}
          title={
            hasInsights
              ? 'Toggle insight badges + implied-FK edges on the graph'
              : 'Open the Insights panel first'
          }
          className={
            'inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40 ' +
            (overlay
              ? 'bg-black/10 text-neutral-900 dark:bg-white/15 dark:text-neutral-100'
              : 'text-neutral-500 hover:bg-black/5 dark:hover:bg-white/5')
          }
        >
          <Lightbulb size={13} />
          Insights
        </button>
```

- [ ] **Step 5: Typecheck, lint, verify in browser**

Run: `npm --prefix web run build && npm --prefix web run lint && npm --prefix web run test`
Expected: clean.

Browser verification (REQUIRED — superpowers:verifying-frontend-changes): open the Insights panel (fetch runs), flip the ActionBar toggle on → severity dots appear on affected nodes and a dashed edge appears from `audit_logs` to `users`; toggle off → both disappear; disconnect/reconnect → overlay state resets.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/Graph/nodeRenderer.ts web/src/components/Graph/GraphCanvas.tsx web/src/App.tsx web/src/components/ActionBar/index.tsx
git commit -m "feat: graph insights overlay — severity badges + implied-FK edges + toggle"
```

---

### Task 12: Full verification sweep

**Files:** none new — this is the gate before the PR.

- [ ] **Step 1: Backend sweep**

```bash
gofmt -l internal/ && go vet ./...
GOTOOLCHAIN=go1.25.8 /home/elgnas/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64/bin/go \
  test -count=1 ./...
```

Expected: no gofmt output, vet clean, all tests PASS (including testcontainers integration).

- [ ] **Step 2: Frontend sweep**

```bash
npm --prefix web run test && npm --prefix web run lint && npm --prefix web run build
```

Expected: all clean. Do NOT stage anything under `internal/server/dist/`.

- [ ] **Step 3: End-to-end run (verify skill)**

Use the `verify` skill (or manually): seeded Postgres up → `go run . serve --dev` + `npm --prefix web run dev` → connect → Insights tab → confirm the full loop (fetch, findings, click-highlight, overlay, refresh). Also `curl` the endpoint directly and eyeball the JSON:

```bash
curl -s "http://127.0.0.1:7777/api/connections/<id>/insights" | head -c 2000
```

- [ ] **Step 4: Commit the plan doc + push**

```bash
git add -f docs/superpowers/plans/2026-07-16-pg-insights-core.md
git commit -m "docs: add pg-insights phase-1 implementation plan"
git status   # confirm: no dist/, no graphify-out/, pure_test.go still untracked
```

Then follow superpowers:finishing-a-development-branch / superpowers:requesting-code-review per the dev-workflow (PR opens as draft against `master`).

---

## Deliberate deviations from the spec (rationale)

1. **`nodeId` not `node_id`** — the frozen frontend contract requires JSON tags to match `types/graph.ts`, which is camelCase throughout.
2. **`RelationshipGaps` returns `([]Finding, []model.Link)`** — the spec's file table says `[]Finding`, but the spec's own §Architecture requires gap candidates to "emit `model.Link{Inferred: true, Confidence}`"; returning the links alongside the findings is the minimal way to honor that without inventing a link-reconstruction step in the handler.
3. **`Collect` helper** — not in the spec's file table, but keeps the handler thin and makes unsupported-capability degradation unit-testable without a fake engine.
4. **No "never analyzed" health finding** — needs `last_autoanalyze`, which `model.TableStats` doesn't carry and the spec doesn't add. Recorded for phase ② consideration.
