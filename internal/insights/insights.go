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
