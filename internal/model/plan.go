package model

// QueryPlan is the engine-agnostic result of an EXPLAIN, reduced to the ordered
// list of tables the query touches so the frontend can animate the path (§10.3).
type QueryPlan struct {
	Query string        `json:"query"`
	Nodes []ExplainNode `json:"nodes"`
}

// ExplainNode is one step in a query plan, in execution order.
type ExplainNode struct {
	Table string  `json:"table"` // node ID the step reads, if any
	Op    string  `json:"op"`    // e.g. "Seq Scan", "Index Scan", "Hash Join"
	Cost  float64 `json:"cost"`  // estimated total cost
}

// ChangeEvent is emitted by live-mode subscriptions (Phase 2).
type ChangeEvent struct {
	NodeID    string `json:"nodeId"`
	Operation string `json:"operation"` // "insert" | "update" | "delete"
	Timestamp string `json:"timestamp"`
}
