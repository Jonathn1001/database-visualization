package insights

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

func indexGraph() *model.GraphModel {
	return &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.users", Schema: "public", Label: "users", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "email", Type: "string (varchar(255))"},
				}},
			{ID: "public.orders", Schema: "public", Label: "orders", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: uuidType, IsFK: true,
						FKRef: &model.FKRef{Table: "public.users", Column: "id"}},
					{Name: "created_at", Type: "timestamp (timestamptz)"},
				}},
		},
	}
}

func titlesFor(fs []Finding, nodeID string) map[string][]Finding {
	out := map[string][]Finding{}
	for _, f := range fs {
		if f.NodeID == nodeID {
			out[f.Title] = append(out[f.Title], f)
		}
	}
	return out
}

func TestIndexFindingsDuplicates(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "public.users", Index: "users_pkey", Columns: []string{"id"}, IsPrimary: true, IsUnique: true, Scans: 10},
		{NodeID: "public.users", Index: "users_email_key", Columns: []string{"email"}, IsUnique: true, Scans: 5},
		{NodeID: "public.users", Index: "idx_users_email", Columns: []string{"email"}, Scans: 0},
		{NodeID: "public.users", Index: "idx_users_email_dup", Columns: []string{"email"}, Scans: 0},
	}
	fs := IndexFindings(stats, indexGraph())
	dups := titlesFor(fs, "public.users")["Duplicate index"]
	if len(dups) != 2 {
		t.Fatalf("expected 2 duplicate findings, got %d: %+v", len(dups), fs)
	}
	for _, d := range dups {
		if d.Severity != SeverityWarn || d.Meta["duplicateOf"] != "users_email_key" {
			t.Errorf("duplicate finding = %+v", d)
		}
	}
	// Duplicates must not also be reported unused.
	if unused := titlesFor(fs, "public.users")["Unused index"]; len(unused) != 0 {
		t.Errorf("duplicate indexes double-reported as unused: %+v", unused)
	}
}

func TestIndexFindingsRedundantPrefix(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "public.orders", Index: "idx_orders_user", Columns: []string{"user_id"}, Scans: 3},
		{NodeID: "public.orders", Index: "idx_orders_user_created", Columns: []string{"user_id", "created_at"}, Scans: 7},
	}
	fs := IndexFindings(stats, indexGraph())
	red := titlesFor(fs, "public.orders")["Redundant index"]
	if len(red) != 1 {
		t.Fatalf("expected 1 redundant finding, got %+v", fs)
	}
	if red[0].Severity != SeverityInfo || red[0].Meta["index"] != "idx_orders_user" ||
		red[0].Meta["coveredBy"] != "idx_orders_user_created" {
		t.Errorf("redundant finding = %+v", red[0])
	}
}

func TestIndexFindingsUnused(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "public.orders", Index: "idx_orders_user", Columns: []string{"user_id"}, Scans: 0, SizeBytes: 20 << 20},
		{NodeID: "public.orders", Index: "idx_orders_created", Columns: []string{"created_at"}, Scans: 0, SizeBytes: 1 << 20},
		{NodeID: "public.orders", Index: "orders_pkey", Columns: []string{"id"}, IsPrimary: true, IsUnique: true, Scans: 0},
	}
	fs := IndexFindings(stats, indexGraph())
	unused := titlesFor(fs, "public.orders")["Unused index"]
	if len(unused) != 2 {
		t.Fatalf("expected 2 unused findings (pkey excluded), got %+v", fs)
	}
	bySev := map[string]int{}
	for _, u := range unused {
		bySev[u.Severity]++
	}
	if bySev[SeverityWarn] != 1 || bySev[SeverityInfo] != 1 {
		t.Errorf("size-based severity split wrong: %+v", unused)
	}
}

func TestIndexFindingsMissingFKIndex(t *testing.T) {
	// No index on orders.user_id at all.
	stats := []IndexStat{
		{NodeID: "public.orders", Index: "orders_pkey", Columns: []string{"id"}, IsPrimary: true, IsUnique: true, Scans: 1},
	}
	fs := IndexFindings(stats, indexGraph())
	missing := titlesFor(fs, "public.orders")["Foreign key without index"]
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing-FK-index finding, got %+v", fs)
	}
	if missing[0].Severity != SeverityWarn || missing[0].Meta["column"] != "user_id" ||
		missing[0].Meta["refTable"] != "public.users" {
		t.Errorf("missing-FK finding = %+v", missing[0])
	}

	// An index whose FIRST column is user_id satisfies the FK.
	stats = append(stats, IndexStat{
		NodeID: "public.orders", Index: "idx_orders_user", Columns: []string{"user_id"}, Scans: 1,
	})
	fs = IndexFindings(stats, indexGraph())
	if missing := titlesFor(fs, "public.orders")["Foreign key without index"]; len(missing) != 0 {
		t.Errorf("indexed FK still reported: %+v", missing)
	}
}

func TestIndexFindingsIgnoresUnknownNodes(t *testing.T) {
	stats := []IndexStat{
		{NodeID: "other.ghost", Index: "idx_ghost", Columns: []string{"x"}, Scans: 0},
	}
	for _, f := range IndexFindings(stats, indexGraph()) {
		if f.NodeID == "other.ghost" {
			t.Errorf("finding emitted for node absent from graph: %+v", f)
		}
	}
}
