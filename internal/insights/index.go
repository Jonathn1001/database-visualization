package insights

import (
	"fmt"
	"sort"
	"strings"

	"github.com/elgnas/dbviz/internal/model"
)

// unusedWarnBytes escalates an unused-index finding from info to warn when the
// wasted space is significant.
const unusedWarnBytes = 10 << 20 // 10 MiB

// IndexFindings analyzes bulk index statistics against the graph: duplicate
// and redundant (prefix) indexes, never-scanned indexes, and FK columns with
// no supporting index. Stats for tables absent from the graph are ignored.
func IndexFindings(stats []IndexStat, g *model.GraphModel) []Finding {
	nodes := map[string]*model.Node{}
	for i := range g.Nodes {
		nodes[g.Nodes[i].ID] = &g.Nodes[i]
	}

	byNode := map[string][]IndexStat{}
	for _, s := range stats {
		if _, ok := nodes[s.NodeID]; ok {
			byNode[s.NodeID] = append(byNode[s.NodeID], s)
		}
	}

	var findings []Finding
	for nodeID, idxs := range byNode {
		reported := map[string]bool{} // index name -> already has a dup/redundant finding

		// --- Duplicates: identical column lists. ---
		groups := map[string][]IndexStat{}
		for _, s := range idxs {
			if s.IsPartial || len(s.Columns) == 0 {
				continue
			}
			key := strings.Join(s.Columns, "\x00")
			groups[key] = append(groups[key], s)
		}
		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			sort.Slice(group, func(i, j int) bool { return keeperRank(group[i]) < keeperRank(group[j]) })
			keeper := group[0]
			for _, dup := range group[1:] {
				if dup.IsPrimary || dup.IsUnique {
					continue // constraint-backed indexes are never droppable duplicates
				}
				reported[dup.Index] = true
				findings = append(findings, Finding{
					Category: CategoryIndex,
					Severity: SeverityWarn,
					NodeID:   nodeID,
					Title:    "Duplicate index",
					Detail: fmt.Sprintf("%s duplicates %s on (%s)",
						dup.Index, keeper.Index, strings.Join(dup.Columns, ", ")),
					Meta: map[string]any{
						"index":       dup.Index,
						"duplicateOf": keeper.Index,
						"columns":     dup.Columns,
						"sizeBytes":   dup.SizeBytes,
					},
				})
			}
		}

		// --- Redundant: strict column-list prefix of a wider index. ---
		for _, a := range idxs {
			if a.IsPrimary || a.IsUnique || a.IsPartial || len(a.Columns) == 0 || reported[a.Index] {
				continue
			}
			for _, b := range idxs {
				if a.Index == b.Index || b.IsPartial || len(b.Columns) <= len(a.Columns) {
					continue
				}
				if isPrefix(a.Columns, b.Columns) {
					reported[a.Index] = true
					findings = append(findings, Finding{
						Category: CategoryIndex,
						Severity: SeverityInfo,
						NodeID:   nodeID,
						Title:    "Redundant index",
						Detail: fmt.Sprintf("%s (%s) is a prefix of %s (%s)",
							a.Index, strings.Join(a.Columns, ", "),
							b.Index, strings.Join(b.Columns, ", ")),
						Meta: map[string]any{
							"index":     a.Index,
							"coveredBy": b.Index,
							"sizeBytes": a.SizeBytes,
						},
					})
					break
				}
			}
		}

		// --- Unused: zero scans since the stats were last reset. ---
		for _, s := range idxs {
			if s.Scans != 0 || s.IsPrimary || s.IsUnique || s.IsPartial || reported[s.Index] {
				continue
			}
			sev := SeverityInfo
			if s.SizeBytes >= unusedWarnBytes {
				sev = SeverityWarn
			}
			findings = append(findings, Finding{
				Category: CategoryIndex,
				Severity: sev,
				NodeID:   nodeID,
				Title:    "Unused index",
				Detail:   fmt.Sprintf("%s has 0 scans since statistics were last reset", s.Index),
				Meta: map[string]any{
					"index":     s.Index,
					"scans":     s.Scans,
					"sizeBytes": s.SizeBytes,
				},
			})
		}
	}

	// --- FK columns with no supporting index (first key column). ---
	for _, n := range g.Nodes {
		firstCols := map[string]bool{}
		for _, s := range byNode[n.ID] {
			if len(s.Columns) > 0 {
				firstCols[s.Columns[0]] = true
			}
		}
		for _, c := range n.Columns {
			if !c.IsFK || firstCols[c.Name] {
				continue
			}
			refTable := ""
			if c.FKRef != nil {
				refTable = c.FKRef.Table
			}
			findings = append(findings, Finding{
				Category: CategoryIndex,
				Severity: SeverityWarn,
				NodeID:   n.ID,
				Title:    "Foreign key without index",
				Detail: fmt.Sprintf("column %s references %s but has no supporting index",
					c.Name, refTable),
				Meta: map[string]any{
					"column":   c.Name,
					"refTable": refTable,
				},
			})
		}
	}

	sortFindings(findings)
	return findings
}

// keeperRank picks which member of a duplicate group survives:
// primary beats unique beats lexicographically-first name.
func keeperRank(s IndexStat) string {
	switch {
	case s.IsPrimary:
		return "0" + s.Index
	case s.IsUnique:
		return "1" + s.Index
	default:
		return "2" + s.Index
	}
}

func isPrefix(short, long []string) bool {
	if len(short) >= len(long) {
		return false
	}
	for i := range short {
		if short[i] != long[i] {
			return false
		}
	}
	return true
}
