package insights

import (
	"fmt"

	"github.com/elgnas/dbviz/internal/model"
)

// Table-health thresholds. Small tables are exempt so a handful of dead
// tuples on a tiny table does not spam findings.
const (
	deadRatioWarn     = 0.20
	deadRatioCritical = 0.50
	deadRatioMinRows  = 1000 // live+dead below this -> skip ratio + vacuum checks

	seqScanMinScans    = 100
	seqScanMinRows     = 10_000
	seqScanRatioFactor = 10
)

// HealthFindings analyzes per-table statistics: dead-tuple ratio (with a
// bloat estimate), never-vacuumed tables, and seq-scan-heavy tables. Callers
// prefilter stats to nodes present in the graph.
func HealthFindings(stats []model.TableStats) []Finding {
	var findings []Finding
	for _, s := range stats {
		total := s.RowCount + s.DeadTuples

		if total >= deadRatioMinRows && s.DeadTuples > 0 {
			ratio := float64(s.DeadTuples) / float64(total)
			if ratio >= deadRatioWarn {
				sev := SeverityWarn
				if ratio >= deadRatioCritical {
					sev = SeverityCritical
				}
				findings = append(findings, Finding{
					Category: CategoryHealth,
					Severity: sev,
					NodeID:   s.NodeID,
					Title:    "High dead-tuple ratio",
					Detail: fmt.Sprintf("%.0f%% of tuples are dead (%d of %d)",
						ratio*100, s.DeadTuples, total),
					Meta: map[string]any{
						"deadTuples":     s.DeadTuples,
						"liveTuples":     s.RowCount,
						"ratio":          ratio,
						"estBloatBytes":  int64(ratio * float64(s.SizeBytes)),
						"lastVacuum":     s.LastVacuum,
						"lastAutovacuum": s.LastAutovacuum,
					},
				})
			}

			if s.LastVacuum == "" && s.LastAutovacuum == "" {
				findings = append(findings, Finding{
					Category: CategoryHealth,
					Severity: SeverityInfo,
					NodeID:   s.NodeID,
					Title:    "Never vacuumed",
					Detail:   "table has dead tuples but has never been vacuumed (manual or auto)",
					Meta: map[string]any{
						"deadTuples": s.DeadTuples,
					},
				})
			}
		}

		if s.SeqScans >= seqScanMinScans && s.RowCount >= seqScanMinRows &&
			(s.IdxScans == 0 || s.SeqScans >= seqScanRatioFactor*s.IdxScans) {
			findings = append(findings, Finding{
				Category: CategoryHealth,
				Severity: SeverityWarn,
				NodeID:   s.NodeID,
				Title:    "Sequential-scan heavy",
				Detail: fmt.Sprintf("%d sequential scans vs %d index scans",
					s.SeqScans, s.IdxScans),
				Meta: map[string]any{
					"seqScans": s.SeqScans,
					"idxScans": s.IdxScans,
					"rowCount": s.RowCount,
				},
			})
		}
	}
	sortFindings(findings)
	return findings
}
