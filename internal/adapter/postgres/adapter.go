// Package postgres implements a read-only IDatabaseAdapter for PostgreSQL using
// pgx/v5 (§10). It enforces read-only sessions and refuses superuser
// credentials (§22.1).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

const engineName = "postgres"

func init() {
	adapter.Register(engineName, func() adapter.IDatabaseAdapter {
		return &Adapter{}
	})
}

// Adapter is a read-only PostgreSQL adapter.
type Adapter struct {
	pool     *pgxpool.Pool
	database string
}

// Engine returns the adapter's engine identifier.
func (a *Adapter) Engine() string { return engineName }

// Open establishes the connection pool, applies read-only session defaults, and
// rejects superuser credentials (§10.2, §22.1).
func (a *Adapter) Open(ctx context.Context, cfg model.ConnectionConfig) error {
	connStr := cfg.DSN
	if connStr == "" {
		connStr = buildConnString(cfg)
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return model.NewAPIError(model.ErrConnInvalidDSN, "invalid Postgres DSN").
			WithHint("expected postgres://user:pass@host:port/dbname")
	}
	poolCfg.MaxConns = 4
	// Apply read-only session defaults on every pooled connection (§10.2).
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `
			SET default_transaction_read_only = on;
			SET default_transaction_isolation = 'repeatable read';
			SET statement_timeout = '30s';
		`)
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return classifyConnError(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return classifyConnError(err)
	}

	// Refuse superuser credentials — too much blast radius for a viz tool.
	var isSuper string
	if err := pool.QueryRow(ctx, `SELECT current_setting('is_superuser')`).Scan(&isSuper); err == nil {
		if strings.EqualFold(isSuper, "on") {
			pool.Close()
			return superuserRejected(poolCfg.ConnConfig.Database)
		}
	}

	a.pool = pool
	a.database = poolCfg.ConnConfig.Database
	return nil
}

// Close releases the connection pool.
func (a *Adapter) Close() error {
	if a.pool != nil {
		a.pool.Close()
		a.pool = nil
	}
	return nil
}

// Ping checks liveness.
func (a *Adapter) Ping(ctx context.Context) error {
	if a.pool == nil {
		return adapter.ErrNotOpen
	}
	return a.pool.Ping(ctx)
}

// Schemas lists user-visible schemas (§10.1, §11).
func (a *Adapter) Schemas(ctx context.Context) ([]string, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	rows, err := a.pool.Query(ctx, querySchemas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	return schemas, rows.Err()
}

// SampleData returns up to limit rows from the given node (table). PII masking
// is applied by the handler layer, not here (§22.3).
func (a *Adapter) SampleData(ctx context.Context, nodeID string, limit int) ([]map[string]any, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	schema, table, err := splitNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 5
	}

	// Identifier sanitation prevents injection; values are never interpolated.
	ident := pgx.Identifier{schema, table}.Sanitize()
	sql := fmt.Sprintf("SELECT * FROM %s LIMIT $1", ident)

	rows, err := a.pool.Query(ctx, sql, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	var out []map[string]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(fields))
		for i, f := range fields {
			row[string(f.Name)] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SubscribeChanges is Phase 2; not supported in the MVP.
func (a *Adapter) SubscribeChanges(ctx context.Context) (<-chan model.ChangeEvent, error) {
	return nil, adapter.ErrOperationNotSupported
}

func buildConnString(cfg model.ConnectionConfig) string {
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		cfg.User, cfg.Password, host, port, cfg.Database)
}

func splitNodeID(nodeID string) (schema, table string, err error) {
	idx := strings.IndexByte(nodeID, '.')
	if idx < 0 {
		return "", "", model.NewAPIError(model.ErrBadRequest, "node ID must be <schema>.<table>")
	}
	return nodeID[:idx], nodeID[idx+1:], nil
}

func classifyConnError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case strings.HasPrefix(pgErr.Code, "28"): // 28000 / 28P01 — auth
			return model.NewAPIError(model.ErrConnAuthFailed, "authentication failed")
		case strings.HasPrefix(pgErr.Code, "3D"): // 3D000 — db does not exist
			return model.NewAPIError(model.ErrConnUnreachable, pgErr.Message)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return model.NewAPIError(model.ErrConnTimeout, "connection timed out")
	}
	return model.NewAPIError(model.ErrConnUnreachable, err.Error()).
		WithHint("check host, port, and that the database is reachable")
}

func superuserRejected(db string) error {
	return model.NewAPIError(model.ErrConnSuperuserRejected, "Refusing to connect with superuser credentials").
		WithHint("Create a least-privilege role for read-only access").
		WithDetails(map[string]string{
			"sql": fmt.Sprintf(`CREATE ROLE dbviz_reader LOGIN PASSWORD 'changeme';
GRANT CONNECT ON DATABASE %s TO dbviz_reader;
GRANT USAGE ON SCHEMA public TO dbviz_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbviz_reader;
GRANT pg_read_all_stats TO dbviz_reader;`, db),
		})
}
