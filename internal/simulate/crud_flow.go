package simulate

import "github.com/elgnas/dbviz/internal/model"

// CrudFlowStep is one node visited by a simulated CRUD operation.
type CrudFlowStep struct {
	NodeID    string `json:"nodeId"`
	Order     int    `json:"order"`
	Action    string `json:"action"` // "insert" | "update" | "delete" | "check"
	Via       string `json:"via,omitempty"`
	ViaColumn string `json:"viaColumn,omitempty"`
}

// CrudFlowResult is the ordered set of steps a CRUD operation on Root implies.
// It is graph-derived: no rows are read or written (read-only tool, §1.4).
type CrudFlowResult struct {
	Operation string         `json:"operation"`
	Root      string         `json:"root"`
	Steps     []CrudFlowStep `json:"steps"`
}

// CRUD action strings.
const (
	ActionInsert = "insert"
	ActionUpdate = "update"
	ActionDelete = "delete"
	ActionCheck  = "check"
)

// CrudFlow computes the chain of steps that performing operation on root would
// involve, derived purely from the graph (§6.3).
//
//   - "delete": the root is deleted first, then every table reached by an
//     ON DELETE CASCADE link, ordered by cascade depth (reuses Cascade).
//   - "insert": parent rows referenced by foreign keys must exist before the
//     root row can be inserted, so each distinct parent is emitted as a "check"
//     step in dependency order (deepest ancestor first), then the root is the
//     final "insert" step. FK cycles are guarded against.
//   - "update": a single "update" step on the root.
func CrudFlow(g *model.GraphModel, root, operation string) CrudFlowResult {
	res := CrudFlowResult{Operation: operation, Root: root}

	switch operation {
	case ActionDelete:
		res.Steps = append(res.Steps, CrudFlowStep{NodeID: root, Order: 0, Action: ActionDelete})
		for i, c := range Cascade(g, root).Affected {
			res.Steps = append(res.Steps, CrudFlowStep{
				NodeID:    c.NodeID,
				Order:     i + 1,
				Action:    ActionDelete,
				Via:       c.Via,
				ViaColumn: c.ViaColumn,
			})
		}

	case ActionInsert:
		parents := insertParents(g, root)
		for i, p := range parents {
			res.Steps = append(res.Steps, CrudFlowStep{
				NodeID:    p.nodeID,
				Order:     i,
				Action:    ActionCheck,
				Via:       p.via,
				ViaColumn: p.viaColumn,
			})
		}
		res.Steps = append(res.Steps, CrudFlowStep{
			NodeID: root,
			Order:  len(parents),
			Action: ActionInsert,
		})

	case ActionUpdate:
		res.Steps = append(res.Steps, CrudFlowStep{NodeID: root, Order: 0, Action: ActionUpdate})
	}

	return res
}

// parentRef is a FK target discovered while walking ancestors of the root.
type parentRef struct {
	nodeID    string
	via       string // child node whose FK points at this parent
	viaColumn string // FK column on the child
}

// insertParents returns the distinct FK targets reachable from root in
// dependency order: deepest ancestor first, the root's direct parents last.
// Cycles are broken by tracking visited nodes; each parent appears once, with
// the Via/ViaColumn of the first edge that reached it.
func insertParents(g *model.GraphModel, root string) []parentRef {
	nodes := nodeIndex(g)

	visited := map[string]bool{root: true}
	// firstRef captures the edge that first reached a parent (for Via metadata).
	firstRef := map[string]parentRef{}

	// Post-order DFS so that a parent is emitted only after its own parents,
	// yielding deepest-ancestor-first ordering.
	var ordered []string
	var walk func(node string)
	walk = func(node string) {
		n, ok := nodes[node]
		if !ok {
			return
		}
		for _, col := range n.Columns {
			if col.FKRef == nil || col.FKRef.Table == "" {
				continue
			}
			parent := col.FKRef.Table
			if parent == node {
				continue // self-reference: a row can satisfy its own FK
			}
			if _, seen := firstRef[parent]; !seen {
				firstRef[parent] = parentRef{nodeID: parent, via: node, viaColumn: col.Name}
			}
			if visited[parent] {
				continue // cycle or already-walked ancestor
			}
			visited[parent] = true
			walk(parent)
			ordered = append(ordered, parent)
		}
	}
	walk(root)

	out := make([]parentRef, 0, len(ordered))
	for _, p := range ordered {
		out = append(out, firstRef[p])
	}
	return out
}

// nodeIndex builds a lookup of node ID -> node.
func nodeIndex(g *model.GraphModel) map[string]model.Node {
	idx := make(map[string]model.Node, len(g.Nodes))
	for i := range g.Nodes {
		idx[g.Nodes[i].ID] = g.Nodes[i]
	}
	return idx
}
