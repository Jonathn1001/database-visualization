// Package simulate computes graph-derived effects (cascade deletes, query
// paths) from a GraphModel without touching the database (§1.4).
package simulate

import "github.com/elgnas/dbviz/internal/model"

// CascadeStep is one table reached by a cascade delete.
type CascadeStep struct {
	NodeID    string `json:"nodeId"`
	Via       string `json:"via"`       // parent node ID that cascaded into this one
	ViaColumn string `json:"viaColumn"` // FK column on this node
	Depth     int    `json:"depth"`
}

// CascadeResult is the ordered set of tables affected by deleting from Root.
type CascadeResult struct {
	Root     string        `json:"root"`
	Affected []CascadeStep `json:"affected"`
}

// Cascade computes the chain of tables a delete on root would cascade to,
// following ON DELETE CASCADE links breadth-first. Deleting a referenced
// (Target) row cascades to the referencing (Source) rows, so we traverse
// Target -> Source edges where Link.Cascade is true.
func Cascade(g *model.GraphModel, root string) CascadeResult {
	result := CascadeResult{Root: root}

	type queued struct {
		node  string
		depth int
	}
	visited := map[string]bool{root: true}
	queue := []queued{{root, 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, l := range g.Links {
			if !l.Cascade || l.Target != cur.node || visited[l.Source] {
				continue
			}
			visited[l.Source] = true
			result.Affected = append(result.Affected, CascadeStep{
				NodeID:    l.Source,
				Via:       cur.node,
				ViaColumn: l.ViaColumn,
				Depth:     cur.depth + 1,
			})
			queue = append(queue, queued{l.Source, cur.depth + 1})
		}
	}
	return result
}
