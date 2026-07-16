package insights

import (
	"context"

	"github.com/elgnas/dbviz/internal/model"
)

// Collect runs the requested insight categories over the graph, sourcing
// engine statistics from src's optional capabilities. category "" means all;
// a capability src does not implement degrades that category to
// status "unsupported" — never an error. Capability query errors propagate.
func Collect(ctx context.Context, g *model.GraphModel, src any, category string) (Result, error) {
	switch category {
	case "", CategoryIndex, CategoryHealth, CategoryGaps:
	default:
		return Result{}, model.NewAPIError(model.ErrBadRequest,
			"category must be one of index, health, gaps")
	}
	want := func(c string) bool { return category == "" || category == c }

	var res Result

	if want(CategoryIndex) {
		cr := CategoryResult{Category: CategoryIndex, Status: StatusUnsupported, Findings: []Finding{}}
		if statser, ok := src.(IndexStatser); ok {
			stats, err := statser.IndexStats(ctx)
			if err != nil {
				return Result{}, err
			}
			cr.Status = StatusOK
			cr.Findings = ensure(IndexFindings(stats, g))
		}
		res.Categories = append(res.Categories, cr)
	}

	if want(CategoryHealth) {
		cr := CategoryResult{Category: CategoryHealth, Status: StatusUnsupported, Findings: []Finding{}}
		if statser, ok := src.(BulkTableStatser); ok {
			stats, err := statser.AllTableStats(ctx)
			if err != nil {
				return Result{}, err
			}
			cr.Status = StatusOK
			cr.Findings = ensure(HealthFindings(filterToGraph(stats, g)))
		}
		res.Categories = append(res.Categories, cr)
	}

	if want(CategoryGaps) {
		findings, links := RelationshipGaps(g)
		res.Categories = append(res.Categories, CategoryResult{
			Category:     CategoryGaps,
			Status:       StatusOK,
			Findings:     ensure(findings),
			ImpliedLinks: links,
		})
	}

	return res, nil
}

// filterToGraph drops stats for tables that are not nodes in the graph
// (e.g. schemas outside the introspected set).
func filterToGraph(stats []model.TableStats, g *model.GraphModel) []model.TableStats {
	known := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		known[n.ID] = true
	}
	out := stats[:0]
	for _, s := range stats {
		if known[s.NodeID] {
			out = append(out, s)
		}
	}
	return out
}

// ensure guarantees a non-nil slice so JSON serializes findings as [].
func ensure(fs []Finding) []Finding {
	if fs == nil {
		return []Finding{}
	}
	return fs
}
