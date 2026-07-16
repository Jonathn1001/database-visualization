package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/elgnas/dbviz/internal/audit"
	"github.com/elgnas/dbviz/internal/connection"
	"github.com/elgnas/dbviz/internal/insights"
	"github.com/elgnas/dbviz/internal/model"
	"github.com/elgnas/dbviz/internal/simulate"

	// Register the postgres adapter in this test binary.
	_ "github.com/elgnas/dbviz/internal/adapter/postgres"
)

func startPostgresReaderDSN(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	initScript, err := filepath.Abs(filepath.Join("..", "..", "testdata", "ecommerce.sql"))
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
	return u.String()
}

func TestAPIEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	readerDSN := startPostgresReaderDSN(t)

	auditDir := t.TempDir()
	auditLog, err := audit.New(auditDir)
	require.NoError(t, err)
	defer auditLog.Close()

	router := NewRouter(Config{Dev: true}, nil, connection.NewManager(connection.Config{}, nil), auditLog)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// 1. Create connection.
	connID := func() string {
		body, _ := json.Marshal(model.ConnectionConfig{DSN: readerDSN})
		res, err := http.Post(srv.URL+"/api/connections", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var meta connMeta
		require.NoError(t, json.NewDecoder(res.Body).Decode(&meta))
		assert.Equal(t, "postgres", meta.Engine)
		assert.Empty(t, meta.Config.Password, "password must be redacted")
		assert.NotContains(t, meta.Config.DSN, "readonly", "DSN password must be redacted (H1)")
		return meta.ID
	}()

	// 2. Introspect schema.
	t.Run("schema", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/api/connections/" + connID + "/schema")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var g model.GraphModel
		require.NoError(t, json.NewDecoder(res.Body).Decode(&g))
		assert.Equal(t, 9, g.Stats.NodeCount)
		assert.GreaterOrEqual(t, g.Stats.LinkCount, 9)
	})

	// 3. Cascade simulation from users.
	t.Run("cascade", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/api/connections/" + connID + "/simulate/cascade/public.users")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var cr simulate.CascadeResult
		require.NoError(t, json.NewDecoder(res.Body).Decode(&cr))
		affected := map[string]bool{}
		for _, s := range cr.Affected {
			affected[s.NodeID] = true
		}
		for _, want := range []string{"public.orders", "public.order_items", "public.payments", "public.reviews", "public.addresses"} {
			assert.True(t, affected[want], "cascade should reach %s", want)
		}
	})

	// 4. EXPLAIN path.
	t.Run("explain", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"query": "SELECT u.name, o.total FROM users u JOIN orders o ON o.user_id = u.id",
		})
		res, err := http.Post(srv.URL+"/api/connections/"+connID+"/explain", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var plan model.QueryPlan
		require.NoError(t, json.NewDecoder(res.Body).Decode(&plan))
		assert.NotEmpty(t, plan.Nodes)
	})

	// 5. Reject a write query through EXPLAIN.
	t.Run("explain_rejects_write", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"query": "DELETE FROM users"})
		res, err := http.Post(srv.URL+"/api/connections/"+connID+"/explain", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	// 6. Sample data is PII-masked by default.
	t.Run("sample_masked", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/api/connections/" + connID + "/tables/public.users/sample?limit=5")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var body struct {
			Rows   []map[string]any `json:"rows"`
			Masked bool             `json:"masked"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.True(t, body.Masked)
		require.NotEmpty(t, body.Rows, "fixture seeds 2 users")
		for _, row := range body.Rows {
			assert.Equal(t, "***", row["email"], "email column must be fully masked")
		}
	})

	// 7. ?full=true returns unmasked data.
	t.Run("sample_full", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/api/connections/" + connID + "/tables/public.users/sample?full=true")
		require.NoError(t, err)
		defer res.Body.Close()
		var body struct {
			Rows   []map[string]any `json:"rows"`
			Masked bool             `json:"masked"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.False(t, body.Masked)
		emails := map[string]bool{}
		for _, row := range body.Rows {
			if e, ok := row["email"].(string); ok {
				emails[e] = true
			}
		}
		assert.True(t, emails["alice@example.com"] || emails["bob@example.com"], "unmasked emails should appear")
	})

	// 8. Audit log captured an entry per database access.
	t.Run("audit", func(t *testing.T) {
		day := time.Now().UTC().Format("2006-01-02")
		f, err := os.Open(filepath.Join(auditDir, "audit-"+day+".jsonl"))
		require.NoError(t, err)
		defer f.Close()

		ops := map[string]bool{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var e audit.Entry
			require.NoError(t, json.Unmarshal(sc.Bytes(), &e))
			ops[e.Operation] = true
		}
		for _, want := range []string{audit.OpIntrospect, audit.OpExplain, audit.OpSample} {
			assert.True(t, ops[want], "audit log should contain a %q entry", want)
		}
	})

	// 9. Delete connection.
	t.Run("delete", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/connections/"+connID, nil)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusNoContent, res.StatusCode)
	})
}

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
