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
