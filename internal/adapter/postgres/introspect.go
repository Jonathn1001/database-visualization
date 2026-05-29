package postgres

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// Default schema shown when the caller does not specify any (§11.2).
var defaultSchemas = []string{"public"}

// Soft and hard limits (§16.1).
const (
	softNodeWarnThreshold = 200
	maxColumnsPerNode     = 100
)

// SQL queries — validated against Postgres 14+ (§10.1).
const (
	querySchemas = `
SELECT nspname AS schema_name
FROM pg_namespace
WHERE nspname NOT IN ('pg_catalog','information_schema')
  AND nspname NOT LIKE 'pg_%'
ORDER BY nspname;`

	queryTables = `
SELECT
    n.nspname AS schema, c.relname AS table_name,
    c.reltuples::bigint AS approx_rows,
    pg_total_relation_size(c.oid) AS size_bytes,
    c.relkind::text AS relkind
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p','v')
  AND n.nspname = ANY($1::text[])
ORDER BY n.nspname, c.relname;`

	// Sourced from pg_catalog rather than information_schema: the latter's
	// constraint views (key_column_usage) are privilege/ownership-filtered, so a
	// least-privilege reader sees no primary keys through them. pg_catalog is
	// readable regardless of ownership. This also avoids a per-column correlated
	// subquery — PKs are joined set-based via pg_index.indisprimary.
	queryColumns = `
SELECT
    ns.nspname                            AS table_schema,
    c.relname                             AS table_name,
    a.attname                             AS column_name,
    format_type(a.atttypid, a.atttypmod)  AS data_type,
    t.typname                             AS udt_name,
    (t.typtype = 'e')                     AS is_enum,
    NOT a.attnotnull                      AS is_nullable,
    pg_get_expr(ad.adbin, ad.adrelid)     AS column_default,
    COALESCE(pk.is_pk, false)             AS is_pk
FROM pg_attribute a
JOIN pg_class c       ON c.oid = a.attrelid
JOIN pg_namespace ns  ON ns.oid = c.relnamespace
JOIN pg_type t        ON t.oid = a.atttypid
LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
LEFT JOIN (
    SELECT i.indrelid, k.attnum, true AS is_pk
    FROM pg_index i
    JOIN LATERAL unnest(i.indkey) AS k(attnum) ON true
    WHERE i.indisprimary
) pk ON pk.indrelid = a.attrelid AND pk.attnum = a.attnum
WHERE c.relkind IN ('r','p','v')
  AND ns.nspname = ANY($1::text[])
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY ns.nspname, c.relname, a.attnum;`

	// Sourced from pg_catalog rather than information_schema: the latter's
	// constraint_column_usage view is filtered to objects the current role owns,
	// so a least-privilege reader (§10.2) sees zero FKs through it. pg_catalog is
	// readable regardless of ownership. unnest pairs handle composite keys.
	queryForeignKeys = `
SELECT
    src_ns.nspname           AS table_schema,
    src.relname              AS table_name,
    src_att.attname          AS column_name,
    tgt_ns.nspname           AS foreign_schema,
    tgt.relname              AS foreign_table,
    tgt_att.attname          AS foreign_column,
    con.confdeltype::text    AS delete_rule,
    con.confupdtype::text    AS update_rule
FROM pg_constraint con
JOIN pg_class src        ON src.oid = con.conrelid
JOIN pg_namespace src_ns ON src_ns.oid = src.relnamespace
JOIN pg_class tgt        ON tgt.oid = con.confrelid
JOIN pg_namespace tgt_ns ON tgt_ns.oid = tgt.relnamespace
JOIN LATERAL unnest(con.conkey)  WITH ORDINALITY AS sk(attnum, ord) ON true
JOIN LATERAL unnest(con.confkey) WITH ORDINALITY AS tk(attnum, ord) ON sk.ord = tk.ord
JOIN pg_attribute src_att ON src_att.attrelid = con.conrelid  AND src_att.attnum = sk.attnum
JOIN pg_attribute tgt_att ON tgt_att.attrelid = con.confrelid AND tgt_att.attnum = tk.attnum
WHERE con.contype = 'f'
  AND src_ns.nspname = ANY($1::text[]);`

	// Joined entirely through OIDs (indexrelid -> indrelid -> namespace) so index
	// names are scoped to their schema. Matching pg_class.relname = indexname
	// (as the pg_indexes view exposes) collides across schemas that reuse names.
	queryIndexColumns = `
SELECT
    ns.nspname AS schemaname,
    t.relname  AS tablename,
    ic.relname AS indexname,
    a.attname  AS column_name
FROM pg_index i
JOIN pg_class ic     ON ic.oid = i.indexrelid
JOIN pg_class t      ON t.oid = i.indrelid
JOIN pg_namespace ns ON ns.oid = t.relnamespace
JOIN pg_attribute a  ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE ns.nspname = ANY($1::text[]);`
)

type columnRow struct {
	schema, table, name string
	dataType, udtName   string
	isEnum              bool
	nullable            bool
	defaultVal          *string
	isPK                bool
}

type fkRow struct {
	schema, table, column                   string
	foreignSchema, foreignTable, foreignCol string
	deleteRule, updateRule                  string
}

type indexColRow struct {
	schema, table, index, column string
}

// Introspect builds the unified GraphModel for the selected schemas (§10, §16).
func (a *Adapter) Introspect(ctx context.Context, opts adapter.IntrospectOptions) (*model.GraphModel, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	schemas := opts.Schemas
	if len(schemas) == 0 {
		schemas = defaultSchemas
	}

	g := &model.GraphModel{
		Engine:   engineName,
		Database: a.database,
		Schemas:  schemas,
	}

	// 1. Tables first — needed for the MaxNodes gate and node existence.
	nodes, nodeIdx, err := a.fetchTables(ctx, schemas)
	if err != nil {
		return nil, introspectErr(err)
	}
	if opts.MaxNodes > 0 && len(nodes) > opts.MaxNodes {
		return nil, a.tooLargeErr(ctx, len(nodes), opts.MaxNodes)
	}

	// 2. Fetch detail sets concurrently (bounded — §10.1).
	var (
		cols    []columnRow
		fks     []fkRow
		idxCols []indexColRow
	)
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(4)
	eg.Go(func() error { var e error; cols, e = a.fetchColumns(egCtx, schemas); return e })
	eg.Go(func() error { var e error; fks, e = a.fetchForeignKeys(egCtx, schemas); return e })
	eg.Go(func() error { var e error; idxCols, e = a.fetchIndexColumns(egCtx, schemas); return e })
	if err := eg.Wait(); err != nil {
		return nil, introspectErr(err)
	}

	// 3. Assemble.
	assembleColumns(nodes, nodeIdx, cols)
	assembleIndexes(nodes, nodeIdx, idxCols)
	links := assembleForeignKeys(nodes, nodeIdx, fks)

	for i := range nodes {
		if len(nodes[i].Columns) > maxColumnsPerNode {
			nodes[i].Columns = nodes[i].Columns[:maxColumnsPerNode]
			g.Warnings = append(g.Warnings, model.Warning{
				Code:    model.WarnColumnsTruncated,
				Message: fmt.Sprintf("%s has more than %d columns; list truncated", nodes[i].ID, maxColumnsPerNode),
			})
		}
	}

	g.Nodes = nodes
	g.Links = links
	g.Stats = computeStats(nodes, links)

	if g.Stats.NodeCount > softNodeWarnThreshold {
		g.Warnings = append(g.Warnings, model.Warning{
			Code:    model.WarnNodeCountHigh,
			Message: fmt.Sprintf("%d nodes — rendering may degrade", g.Stats.NodeCount),
		})
	}
	return g, nil
}

func (a *Adapter) fetchTables(ctx context.Context, schemas []string) ([]model.Node, map[string]int, error) {
	rows, err := a.pool.Query(ctx, queryTables, schemas)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	idx := map[string]int{}
	for rows.Next() {
		var schema, table, relkind string
		var approxRows, sizeBytes int64
		if err := rows.Scan(&schema, &table, &approxRows, &sizeBytes, &relkind); err != nil {
			return nil, nil, err
		}
		if approxRows < 0 {
			approxRows = 0 // reltuples is -1 before first ANALYZE
		}
		kind := model.KindTable
		if relkind == "v" {
			kind = model.KindView
		}
		id := schema + "." + table
		idx[id] = len(nodes)
		nodes = append(nodes, model.Node{
			ID:        id,
			Schema:    schema,
			Label:     table,
			Kind:      kind,
			RowCount:  approxRows,
			SizeBytes: sizeBytes,
		})
	}
	return nodes, idx, rows.Err()
}

func (a *Adapter) fetchColumns(ctx context.Context, schemas []string) ([]columnRow, error) {
	rows, err := a.pool.Query(ctx, queryColumns, schemas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []columnRow
	for rows.Next() {
		var c columnRow
		if err := rows.Scan(&c.schema, &c.table, &c.name, &c.dataType, &c.udtName,
			&c.isEnum, &c.nullable, &c.defaultVal, &c.isPK); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *Adapter) fetchForeignKeys(ctx context.Context, schemas []string) ([]fkRow, error) {
	rows, err := a.pool.Query(ctx, queryForeignKeys, schemas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []fkRow
	for rows.Next() {
		var f fkRow
		if err := rows.Scan(&f.schema, &f.table, &f.column, &f.foreignSchema,
			&f.foreignTable, &f.foreignCol, &f.deleteRule, &f.updateRule); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (a *Adapter) fetchIndexColumns(ctx context.Context, schemas []string) ([]indexColRow, error) {
	rows, err := a.pool.Query(ctx, queryIndexColumns, schemas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []indexColRow
	for rows.Next() {
		var r indexColRow
		if err := rows.Scan(&r.schema, &r.table, &r.index, &r.column); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func assembleColumns(nodes []model.Node, idx map[string]int, cols []columnRow) {
	for _, c := range cols {
		ni, ok := idx[c.schema+"."+c.table]
		if !ok {
			continue
		}
		col := model.Column{
			Name:     c.name,
			Type:     mapType(c.dataType, c.udtName, c.isEnum),
			Nullable: c.nullable,
			IsPK:     c.isPK,
		}
		if c.defaultVal != nil {
			col.DefaultVal = *c.defaultVal
		}
		nodes[ni].Columns = append(nodes[ni].Columns, col)
	}
}

func assembleIndexes(nodes []model.Node, idx map[string]int, idxCols []indexColRow) {
	for _, r := range idxCols {
		ni, ok := idx[r.schema+"."+r.table]
		if !ok {
			continue
		}
		for ci := range nodes[ni].Columns {
			if nodes[ni].Columns[ci].Name == r.column {
				nodes[ni].Columns[ci].Indexes = append(nodes[ni].Columns[ci].Indexes, r.index)
			}
		}
	}
}

func assembleForeignKeys(nodes []model.Node, idx map[string]int, fks []fkRow) []model.Link {
	var links []model.Link
	for _, f := range fks {
		srcID := f.schema + "." + f.table
		tgtID := f.foreignSchema + "." + f.foreignTable
		ni, ok := idx[srcID]
		if !ok {
			continue
		}
		// Mark the child column as a FK.
		for ci := range nodes[ni].Columns {
			if nodes[ni].Columns[ci].Name == f.column {
				nodes[ni].Columns[ci].IsFK = true
				nodes[ni].Columns[ci].FKRef = &model.FKRef{
					Table:    tgtID,
					Column:   f.foreignCol,
					OnDelete: mapFKAction(f.deleteRule),
					OnUpdate: mapFKAction(f.updateRule),
				}
			}
		}
		links = append(links, model.Link{
			Source:    srcID,
			Target:    tgtID,
			Type:      model.Link1ToN,
			Cascade:   mapFKAction(f.deleteRule) == model.OnCascade,
			ViaColumn: f.column,
		})
	}
	return links
}

// mapFKAction maps a pg_constraint confdeltype/confupdtype char code to a
// canonical referential action.
func mapFKAction(code string) string {
	switch code {
	case "c":
		return model.OnCascade
	case "n":
		return model.OnSetNull
	case "r":
		return model.OnRestrict
	case "d":
		return "SET DEFAULT"
	default: // "a" — no action
		return model.OnNoAction
	}
}

func computeStats(nodes []model.Node, links []model.Link) model.Stats {
	colCount := 0
	for _, n := range nodes {
		colCount += len(n.Columns)
	}
	return model.Stats{
		NodeCount:         len(nodes),
		LinkCount:         len(links),
		ColumnCount:       colCount,
		CascadeChainDepth: cascadeDepth(links),
	}
}

// cascadeDepth returns the longest chain of ON DELETE CASCADE relationships.
// A delete on a referenced (Target) table cascades to the referencing (Source)
// table, so we walk Target -> Source edges.
func cascadeDepth(links []model.Link) int {
	adj := map[string][]string{}
	for _, l := range links {
		if l.Cascade {
			adj[l.Target] = append(adj[l.Target], l.Source)
		}
	}
	memo := map[string]int{}
	var depth func(node string, path map[string]bool) int
	depth = func(node string, path map[string]bool) int {
		if d, ok := memo[node]; ok {
			return d
		}
		if path[node] {
			return 0 // cycle guard
		}
		path[node] = true
		best := 0
		for _, child := range adj[node] {
			if d := depth(child, path); d+1 > best {
				best = d + 1
			}
		}
		path[node] = false
		memo[node] = best
		return best
	}
	max := 0
	for node := range adj {
		if d := depth(node, map[string]bool{}); d > max {
			max = d
		}
	}
	return max
}

func (a *Adapter) tooLargeErr(ctx context.Context, count, limit int) error {
	available, _ := a.Schemas(ctx)
	return model.NewAPIError(model.ErrSchemaTooLarge,
		fmt.Sprintf("Schema has %d tables — exceeds limit of %d", count, limit)).
		WithHint("Filter to specific schemas, or set DBVIZ_MAX_NODES env var").
		WithDetails(map[string]any{
			"nodeCount":        count,
			"limit":            limit,
			"availableSchemas": available,
		})
}

func introspectErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
}
