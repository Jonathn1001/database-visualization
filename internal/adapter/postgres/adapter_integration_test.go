package postgres

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/adapter/conformance"
	"github.com/elgnas/dbviz/internal/model"
)

// startPostgres spins up a seeded postgres:16-alpine container and returns the
// superuser DSN and the least-privilege reader DSN.
func startPostgres(t *testing.T) (superDSN, readerDSN string) {
	t.Helper()
	ctx := context.Background()

	initScript, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "ecommerce.sql"))
	require.NoError(t, err)

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithInitScripts(initScript),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	super, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	u, err := url.Parse(super)
	require.NoError(t, err)
	u.User = url.UserPassword("dbviz_reader", "readonly")
	return super, u.String()
}

func TestPostgresConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	_, readerDSN := startPostgres(t)

	conformance.RunSuite(t, func() adapter.IDatabaseAdapter { return &Adapter{} }, conformance.Options{
		Config:       model.ConnectionConfig{Engine: "postgres", DSN: readerDSN},
		ExplainQuery: "SELECT u.name, o.total FROM users u JOIN orders o ON o.user_id = u.id",
		MinNodes:     9,
		AttemptWrite: func(ctx context.Context, a adapter.IDatabaseAdapter) error {
			pa := a.(*Adapter)
			_, err := pa.pool.Exec(ctx,
				"INSERT INTO users(id,email) VALUES (gen_random_uuid(),'x@y.z')")
			return err
		},
	})
}

func TestPostgresIntrospectShape(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	_, readerDSN := startPostgres(t)
	ctx := context.Background()

	a := &Adapter{}
	require.NoError(t, a.Open(ctx, model.ConnectionConfig{Engine: "postgres", DSN: readerDSN}))
	defer a.Close()

	g, err := a.Introspect(ctx, adapter.IntrospectOptions{})
	require.NoError(t, err)

	assert.Equal(t, 9, g.Stats.NodeCount, "ecommerce fixture has 9 tables")
	assert.GreaterOrEqual(t, g.Stats.LinkCount, 9, "fixture has >= 9 FK links")
	assert.GreaterOrEqual(t, g.Stats.CascadeChainDepth, 2)

	byID := map[string]model.Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	users, ok := byID["public.users"]
	require.True(t, ok, "public.users node present")

	var emailCol, idCol *model.Column
	for i := range users.Columns {
		switch users.Columns[i].Name {
		case "email":
			emailCol = &users.Columns[i]
		case "id":
			idCol = &users.Columns[i]
		}
	}
	require.NotNil(t, emailCol, "users.email column present")
	assert.Contains(t, emailCol.Indexes, "idx_users_email")

	// PK detection must work for the least-privilege reader role (regression
	// guard: information_schema hid PKs from non-owner roles).
	require.NotNil(t, idCol, "users.id column present")
	assert.True(t, idCol.IsPK, "users.id must be detected as primary key")

	pkTotal := 0
	for _, n := range g.Nodes {
		for _, c := range n.Columns {
			if c.IsPK {
				pkTotal++
			}
		}
	}
	assert.Equal(t, 9, pkTotal, "all 9 fixture tables have a single-column PK")
}

func TestPostgresExplainPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	_, readerDSN := startPostgres(t)
	ctx := context.Background()

	a := &Adapter{}
	require.NoError(t, a.Open(ctx, model.ConnectionConfig{Engine: "postgres", DSN: readerDSN}))
	defer a.Close()

	plan, err := a.ExplainQuery(ctx,
		"SELECT u.name, o.total FROM users u JOIN orders o ON o.user_id = u.id")
	require.NoError(t, err)

	tables := map[string]bool{}
	for _, n := range plan.Nodes {
		tables[n.Table] = true
	}
	assert.True(t, tables["public.users"], "plan touches users")
	assert.True(t, tables["public.orders"], "plan touches orders")
}

func TestPostgresRejectsSuperuser(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	superDSN, _ := startPostgres(t)

	a := &Adapter{}
	err := a.Open(context.Background(), model.ConnectionConfig{Engine: "postgres", DSN: superDSN})
	require.Error(t, err)
	var apiErr *model.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, model.ErrConnSuperuserRejected, apiErr.Code)
}
