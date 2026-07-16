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
JOIN pg_index i ON i.indexrelid = s.indexrelid
ORDER BY s.schemaname, s.relname, s.indexrelname;`

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
