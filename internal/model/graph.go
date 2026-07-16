// Package model defines the canonical, engine-agnostic data shapes that every
// adapter normalizes into. GraphModel is the contract between adapters and the
// frontend — see DESIGN_PLAN §2.3.
package model

// GraphModel is the unified schema representation produced by every adapter.
type GraphModel struct {
	Engine   string    `json:"engine"` // "postgres" | "mongodb" | ...
	Database string    `json:"database"`
	Schemas  []string  `json:"schemas"` // e.g. ["public", "audit"] for Postgres
	Nodes    []Node    `json:"nodes"`
	Links    []Link    `json:"links"`
	Stats    Stats     `json:"stats"`
	Warnings []Warning `json:"warnings,omitempty"`
}

// Node is a table, collection, or view in the graph.
type Node struct {
	ID        string   `json:"id"`     // "<schema>.<name>" — see §11
	Schema    string   `json:"schema"` // "public" for Postgres; "" for engines w/o schemas
	Label     string   `json:"label"`  // display name (table name, no schema prefix)
	Kind      string   `json:"kind"`   // "table" | "collection" | "view" | "index_pattern"
	Columns   []Column `json:"columns"`
	RowCount  int64    `json:"rowCount"`
	SizeBytes int64    `json:"sizeBytes"`
	Tenant    string   `json:"tenant,omitempty"` // Phase 2
}

// Column is a single field on a Node.
type Column struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"` // canonical, see §2.4
	Nullable   bool     `json:"nullable"`
	IsPK       bool     `json:"isPk"`
	IsFK       bool     `json:"isFk"`
	FKRef      *FKRef   `json:"fkRef,omitempty"`
	Indexes    []string `json:"indexes,omitempty"`
	DefaultVal string   `json:"default,omitempty"`
}

// FKRef describes a foreign-key relationship from a Column to another node.
type FKRef struct {
	Table      string  `json:"table"` // referenced node ID
	Column     string  `json:"column"`
	OnDelete   string  `json:"onDelete"` // "CASCADE" | "SET NULL" | "RESTRICT" | "NO ACTION"
	OnUpdate   string  `json:"onUpdate"`
	Inferred   bool    `json:"inferred"` // true for NoSQL heuristic
	Confidence float64 `json:"confidence,omitempty"`
}

// Link is an edge between two nodes, derived from FK relationships.
type Link struct {
	Source     string  `json:"source"` // node ID
	Target     string  `json:"target"` // node ID
	Type       string  `json:"type"`   // "1:1" | "1:N" | "M:N"
	Cascade    bool    `json:"cascade"`
	ViaColumn  string  `json:"viaColumn"`
	Inferred   bool    `json:"inferred"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Stats summarizes the graph.
type Stats struct {
	NodeCount         int `json:"nodeCount"`
	LinkCount         int `json:"linkCount"`
	CascadeChainDepth int `json:"cascadeChainDepth"`
	ColumnCount       int `json:"columnCount"`
}

// Warning is a non-fatal advisory attached to a GraphModel.
type Warning struct {
	Code    string `json:"code"` // "NODE_COUNT_HIGH" | "SCHEMA_FILTERED" | ...
	Message string `json:"message"`
}

// Warning codes used in GraphModel.Warnings.
const (
	WarnNodeCountHigh    = "NODE_COUNT_HIGH"
	WarnSchemaFiltered   = "SCHEMA_FILTERED"
	WarnColumnsTruncated = "COLUMNS_TRUNCATED"
)

// Node kinds.
const (
	KindTable        = "table"
	KindCollection   = "collection"
	KindView         = "view"
	KindIndexPattern = "index_pattern"
)

// Link cardinality types.
const (
	Link1To1 = "1:1"
	Link1ToN = "1:N"
	LinkMToN = "M:N"
)

// FK referential actions.
const (
	OnCascade  = "CASCADE"
	OnSetNull  = "SET NULL"
	OnRestrict = "RESTRICT"
	OnNoAction = "NO ACTION"
)

// Canonical column types — every adapter maps engine-specific types to one of
// these (§2.4). The original engine type is preserved as a suffix, e.g.
// "uuid (uuid)", "string (varchar(255))".
const (
	TypeString    = "string"
	TypeText      = "text"
	TypeInteger   = "integer"
	TypeBigint    = "bigint"
	TypeDecimal   = "decimal"
	TypeFloat     = "float"
	TypeBoolean   = "boolean"
	TypeTimestamp = "timestamp"
	TypeDate      = "date"
	TypeUUID      = "uuid"
	TypeJSON      = "json"
	TypeBinary    = "binary"
	TypeArray     = "array"
	TypeEnum      = "enum"
	TypeGeometry  = "geometry"
	TypeUnknown   = "unknown"
)

// TableStats holds per-table statistics surfaced at
// GET /api/connections/:id/tables/:nodeId/stats.
type TableStats struct {
	NodeID         string `json:"nodeId"`
	RowCount       int64  `json:"rowCount"`
	DeadTuples     int64  `json:"deadTuples,omitempty"`
	SizeBytes      int64  `json:"sizeBytes"`
	Inserts        int64  `json:"inserts,omitempty"`
	Updates        int64  `json:"updates,omitempty"`
	Deletes        int64  `json:"deletes,omitempty"`
	SeqScans       int64  `json:"seqScans,omitempty"`
	IdxScans       int64  `json:"idxScans,omitempty"`
	LastVacuum     string `json:"lastVacuum,omitempty"`
	LastAutovacuum string `json:"lastAutovacuum,omitempty"`
	LastAnalyze    string `json:"lastAnalyze,omitempty"`
}
