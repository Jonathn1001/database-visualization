// Package adapter defines the contract every database engine must satisfy and
// the registry/factory used to construct adapters by engine name (§9).
package adapter

import (
	"context"

	"github.com/elgnas/dbviz/internal/model"
)

// IDatabaseAdapter is the interface every engine adapter implements. All methods
// that touch the database take a context and must honor cancellation (§15).
type IDatabaseAdapter interface {
	Open(ctx context.Context, cfg model.ConnectionConfig) error
	Close() error
	Ping(ctx context.Context) error
	Engine() string

	// Introspect honors ctx — if cancelled, it must abort all in-flight
	// queries and return promptly.
	Introspect(ctx context.Context, opts IntrospectOptions) (*model.GraphModel, error)

	// Schemas lists the schemas available to the connection (§11).
	Schemas(ctx context.Context) ([]string, error)

	SampleData(ctx context.Context, nodeID string, limit int) ([]map[string]any, error)
	TableStats(ctx context.Context, nodeID string) (*model.TableStats, error)
	ExplainQuery(ctx context.Context, query string) (*model.QueryPlan, error)

	// SubscribeChanges is Phase 2; adapters may return ErrOperationNotSupported.
	SubscribeChanges(ctx context.Context) (<-chan model.ChangeEvent, error)
}

// IntrospectOptions tunes a single Introspect call.
type IntrospectOptions struct {
	Schemas   []string // empty = adapter default (§11)
	MaxNodes  int      // 0 = no cap; returns SCHEMA_TOO_LARGE if exceeded (§16)
	MaxLinks  int      // 0 = no cap
	SkipStats bool     // skip expensive pg_stat_* queries
}
