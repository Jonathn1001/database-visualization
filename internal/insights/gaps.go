package insights

import (
	"fmt"
	"strings"

	"github.com/elgnas/dbviz/internal/model"
)

// Gap confidence scores (spec: exact name+type match scores higher than a
// derived plural match).
const (
	gapConfidenceExact  = 0.9
	gapConfidencePlural = 0.75
)

// pkTarget is a table with a single-column primary key — a candidate target
// for an implied foreign key.
type pkTarget struct {
	nodeID string
	pkType string
}

// RelationshipGaps finds columns that look like foreign keys but have no
// constraint, using catalog heuristics only (no data sampling). It returns
// one Finding per candidate column plus a matching inferred model.Link per
// finding (same order), reusing the §2.3 inferred-edge contract.
func RelationshipGaps(g *model.GraphModel) ([]Finding, []model.Link) {
	// Index candidate targets by "<schema>|<label>".
	targets := map[string]pkTarget{}
	for _, n := range g.Nodes {
		if n.Kind != model.KindTable {
			continue
		}
		var pk *model.Column
		pkCount := 0
		for i := range n.Columns {
			if n.Columns[i].IsPK {
				pk = &n.Columns[i]
				pkCount++
			}
		}
		if pkCount != 1 {
			continue // composite or missing PK — not a v1 target
		}
		targets[n.Schema+"|"+n.Label] = pkTarget{nodeID: n.ID, pkType: pk.Type}
	}

	var findings []Finding
	linkByKey := map[string]model.Link{} // finding key -> link, to re-emit in sorted order
	for _, n := range g.Nodes {
		if n.Kind != model.KindTable {
			continue
		}
		for _, c := range n.Columns {
			if c.IsPK || c.IsFK || !strings.HasSuffix(c.Name, "_id") {
				continue
			}
			base := strings.TrimSuffix(c.Name, "_id")
			if base == "" {
				continue
			}
			target, conf, ok := bestTarget(targets, n.Schema, base, c.Type)
			if !ok || target.nodeID == n.ID {
				continue
			}
			sev := SeverityInfo
			if conf >= gapConfidenceExact {
				sev = SeverityWarn
			}
			f := Finding{
				Category: CategoryGaps,
				Severity: sev,
				NodeID:   n.ID,
				Title:    "Possible missing foreign key",
				Detail: fmt.Sprintf("%s.%s matches %s but has no foreign-key constraint",
					n.ID, c.Name, target.nodeID),
				Meta: map[string]any{
					"column":       c.Name,
					"targetNodeId": target.nodeID,
					"confidence":   conf,
				},
			}
			findings = append(findings, f)
			linkByKey[gapKey(f)] = model.Link{
				Source:     n.ID,
				Target:     target.nodeID,
				Type:       model.Link1ToN,
				ViaColumn:  c.Name,
				Inferred:   true,
				Confidence: conf,
			}
		}
	}

	sortFindings(findings)
	links := make([]model.Link, 0, len(findings))
	for _, f := range findings {
		links = append(links, linkByKey[gapKey(f)])
	}
	return findings, links
}

func gapKey(f Finding) string {
	return f.NodeID + "|" + f.Meta["column"].(string)
}

// bestTarget picks the highest-confidence same-schema target table whose
// single-column PK type exactly equals colType. Exact label match beats
// plural-derived matches; ties break on node ID.
func bestTarget(targets map[string]pkTarget, schema, base, colType string) (pkTarget, float64, bool) {
	type cand struct {
		label string
		conf  float64
	}
	cands := []cand{{base, gapConfidenceExact}}
	for _, p := range pluralForms(base) {
		cands = append(cands, cand{p, gapConfidencePlural})
	}

	var best pkTarget
	bestConf := 0.0
	found := false
	for _, c := range cands {
		t, ok := targets[schema+"|"+c.label]
		if !ok || t.pkType != colType {
			continue
		}
		if !found || c.conf > bestConf || (c.conf == bestConf && t.nodeID < best.nodeID) {
			best, bestConf, found = t, c.conf, true
		}
	}
	return best, bestConf, found
}

// pluralForms derives naive plural table names from a singular base:
// user -> users, box -> boxes, category -> categories.
func pluralForms(base string) []string {
	forms := []string{base + "s", base + "es"}
	if strings.HasSuffix(base, "y") && len(base) > 1 {
		forms = append(forms, base[:len(base)-1]+"ies")
	}
	return forms
}
