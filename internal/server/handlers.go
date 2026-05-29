package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/audit"
	"github.com/elgnas/dbviz/internal/connection"
	"github.com/elgnas/dbviz/internal/model"
	"github.com/elgnas/dbviz/internal/security"
	"github.com/elgnas/dbviz/internal/simulate"
)

// api holds the dependencies shared by HTTP handlers.
type api struct {
	mgr   *connection.Manager
	log   *slog.Logger
	audit audit.Logger
}

// rec writes an audit entry for a database access (§18). The sql argument must
// never contain interpolated parameter values.
func (a *api) rec(mc *connection.ManagedConnection, op, sql string, start time.Time, rowCount int, err error) {
	if a.audit == nil {
		return
	}
	a.audit.Record(audit.Entry{
		ConnectionID: mc.ID,
		Engine:       mc.Engine,
		Database:     mc.Config.Database,
		Operation:    op,
		SQL:          sql,
		DurationMS:   time.Since(start).Milliseconds(),
		RowCount:     rowCount,
		Error:        audit.ErrString(err),
	})
}

// connMeta is the JSON shape returned for a connection.
type connMeta struct {
	ID       string                 `json:"id"`
	Engine   string                 `json:"engine"`
	State    string                 `json:"state"`
	Config   model.ConnectionConfig `json:"config"`
	OpenedAt time.Time              `json:"openedAt"`
}

func metaOf(mc *connection.ManagedConnection) connMeta {
	return connMeta{
		ID:       mc.ID,
		Engine:   mc.Engine,
		State:    string(mc.State()),
		Config:   mc.Config, // already redacted by the manager
		OpenedAt: mc.OpenedAt,
	}
}

func (a *api) createConnection(w http.ResponseWriter, r *http.Request) {
	var cfg model.ConnectionConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, model.NewAPIError(model.ErrBadRequest, "invalid request body"))
		return
	}
	mc, err := a.mgr.Open(r.Context(), cfg)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, metaOf(mc))
}

func (a *api) listConnections(w http.ResponseWriter, _ *http.Request) {
	conns := a.mgr.List()
	out := make([]connMeta, 0, len(conns))
	for _, mc := range conns {
		out = append(out, metaOf(mc))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) getConnection(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, metaOf(mc))
}

func (a *api) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if err := a.mgr.Close(chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) testConnection(w http.ResponseWriter, r *http.Request) {
	var cfg model.ConnectionConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, model.NewAPIError(model.ErrBadRequest, "invalid request body"))
		return
	}
	mc, err := a.mgr.Open(r.Context(), cfg)
	if err != nil {
		writeError(w, err)
		return
	}
	engine := mc.Engine
	_ = a.mgr.Close(mc.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "engine": engine})
}

func (a *api) pingConnection(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := mc.Adapter.Ping(r.Context()); err != nil {
		writeError(w, model.NewAPIError(model.ErrConnUnreachable, "ping failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) getSchema(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	opts := adapter.IntrospectOptions{
		MaxNodes: maxNodesFromEnv(),
	}
	if s := r.URL.Query().Get("schemas"); s != "" {
		opts.Schemas = splitCSV(s)
	}
	start := time.Now()
	g, err := mc.Adapter.Introspect(r.Context(), opts)
	nodeCount := 0
	if g != nil {
		nodeCount = g.Stats.NodeCount
	}
	a.rec(mc, audit.OpIntrospect, "introspect schema "+strings.Join(opts.Schemas, ","), start, nodeCount, err)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (a *api) listSchemas(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	schemas, err := mc.Adapter.Schemas(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schemas": schemas})
}

func (a *api) sampleData(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	nodeID := chi.URLParam(r, "nodeId")
	start := time.Now()
	rows, err := mc.Adapter.SampleData(r.Context(), nodeID, limit)
	a.rec(mc, audit.OpSample, "SELECT * FROM "+nodeID+" LIMIT $1", start, len(rows), err)
	if err != nil {
		writeError(w, err)
		return
	}
	// Mask PII by default (§22.3). The frontend may request unmasked data with
	// ?full=true after an explicit user confirmation.
	masked := true
	if r.URL.Query().Get("full") == "true" {
		masked = false
	} else {
		rows = security.MaskRows(rows)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "masked": masked})
}

func (a *api) tableStats(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	nodeID := chi.URLParam(r, "nodeId")
	start := time.Now()
	stats, err := mc.Adapter.TableStats(r.Context(), nodeID)
	a.rec(mc, audit.OpStats, "pg_stat_user_tables "+nodeID, start, 0, err)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *api) simulateCascade(w http.ResponseWriter, r *http.Request) {
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
	a.rec(mc, audit.OpIntrospect, "introspect for cascade", start, nodeCount, err)
	if err != nil {
		writeError(w, err)
		return
	}
	res := simulate.Cascade(g, chi.URLParam(r, "nodeId"))
	writeJSON(w, http.StatusOK, res)
}

func (a *api) explain(w http.ResponseWriter, r *http.Request) {
	mc, err := a.mgr.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Query) == "" {
		writeError(w, model.NewAPIError(model.ErrBadRequest, "query is required"))
		return
	}
	start := time.Now()
	plan, err := mc.Adapter.ExplainQuery(r.Context(), body.Query)
	a.rec(mc, audit.OpExplain, "EXPLAIN "+body.Query, start, 0, err)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func maxNodesFromEnv() int {
	if v := os.Getenv("DBVIZ_MAX_NODES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 500 // §16.1 default
}
