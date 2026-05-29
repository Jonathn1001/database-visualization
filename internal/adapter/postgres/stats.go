package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

const queryTableStats = `
SELECT
    s.n_live_tup, s.n_dead_tup, s.n_tup_ins, s.n_tup_upd, s.n_tup_del,
    s.last_vacuum, s.last_autovacuum, s.last_analyze,
    pg_total_relation_size(format('%I.%I', s.schemaname, s.relname)::regclass) AS size_bytes
FROM pg_stat_user_tables s
WHERE s.schemaname = $1 AND s.relname = $2;`

// TableStats returns per-table statistics (§6.2, §10.1).
func (a *Adapter) TableStats(ctx context.Context, nodeID string) (*model.TableStats, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	schema, table, err := splitNodeID(nodeID)
	if err != nil {
		return nil, err
	}

	var (
		live, dead, ins, upd, del, size         int64
		lastVacuum, lastAutovacuum, lastAnalyze *time.Time
	)
	row := a.pool.QueryRow(ctx, queryTableStats, schema, table)
	if err := row.Scan(&live, &dead, &ins, &upd, &del,
		&lastVacuum, &lastAutovacuum, &lastAnalyze, &size); err != nil {
		if err == pgx.ErrNoRows {
			return nil, model.NewAPIError(model.ErrSchemaNotFound, "table not found: "+nodeID)
		}
		return nil, model.NewAPIError(model.ErrSchemaIntrospectFailed, err.Error())
	}

	return &model.TableStats{
		NodeID:         nodeID,
		RowCount:       live,
		DeadTuples:     dead,
		SizeBytes:      size,
		Inserts:        ins,
		Updates:        upd,
		Deletes:        del,
		LastVacuum:     fmtTime(lastVacuum),
		LastAutovacuum: fmtTime(lastAutovacuum),
		LastAnalyze:    fmtTime(lastAnalyze),
	}, nil
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
