package postgres

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

func TestMapType(t *testing.T) {
	cases := []struct {
		formatType, typName string
		isEnum              bool
		wantCanonical       string
	}{
		{"uuid", "uuid", false, model.TypeUUID},
		{"character varying(255)", "varchar", false, model.TypeString},
		{"character varying", "varchar", false, model.TypeString},
		{"text", "text", false, model.TypeText},
		{"integer", "int4", false, model.TypeInteger},
		{"bigint", "int8", false, model.TypeBigint},
		{"numeric(10,2)", "numeric", false, model.TypeDecimal},
		{"boolean", "bool", false, model.TypeBoolean},
		{"timestamp without time zone", "timestamp", false, model.TypeTimestamp},
		{"jsonb", "jsonb", false, model.TypeJSON},
		{"integer[]", "_int4", false, model.TypeArray},
		{"order_status", "order_status", true, model.TypeEnum},
		{"something_weird", "weird", false, model.TypeUnknown},
	}
	for _, c := range cases {
		got := canonicalPGType(c.formatType, c.typName, c.isEnum)
		if got != c.wantCanonical {
			t.Errorf("canonicalPGType(%q,%q,%v) = %q, want %q", c.formatType, c.typName, c.isEnum, got, c.wantCanonical)
		}
	}

	if got := mapType("character varying(255)", "varchar", false); got != "string (character varying(255))" {
		t.Errorf("mapType suffix = %q, want %q", got, "string (character varying(255))")
	}
}

func TestValidateReadOnlyQuery(t *testing.T) {
	ok := []string{
		"SELECT 1",
		"  select * from users",
		"WITH t AS (SELECT 1) SELECT * FROM t",
		"-- comment\nSELECT 1",
	}
	for _, q := range ok {
		if err := validateReadOnlyQuery(q); err != nil {
			t.Errorf("validateReadOnlyQuery(%q) rejected: %v", q, err)
		}
	}
	bad := []string{
		"INSERT INTO users VALUES (1)",
		"UPDATE users SET x=1",
		"DELETE FROM users",
		"DROP TABLE users",
		"TRUNCATE users",
		"",
	}
	for _, q := range bad {
		if err := validateReadOnlyQuery(q); err == nil {
			t.Errorf("validateReadOnlyQuery(%q) accepted, want rejection", q)
		}
	}
}

func TestParseExplainJSON(t *testing.T) {
	raw := []byte(`[
	  {"Plan": {
	    "Node Type": "Hash Join",
	    "Total Cost": 42.5,
	    "Plans": [
	      {"Node Type": "Seq Scan", "Relation Name": "users", "Schema": "public", "Total Cost": 10.0},
	      {"Node Type": "Hash", "Plans": [
	        {"Node Type": "Seq Scan", "Relation Name": "orders", "Schema": "public", "Total Cost": 20.0}
	      ]}
	    ]
	  }}
	]`)
	nodes := parseExplainJSON(raw)
	if len(nodes) != 2 {
		t.Fatalf("got %d relation nodes, want 2", len(nodes))
	}
	if nodes[0].Table != "public.users" || nodes[1].Table != "public.orders" {
		t.Errorf("unexpected order/tables: %+v", nodes)
	}
	if nodes[0].Op != "Seq Scan" {
		t.Errorf("op = %q, want Seq Scan", nodes[0].Op)
	}
}

func TestCascadeDepth(t *testing.T) {
	// users <-(cascade)- orders <-(cascade)- order_items ; products <- order_items (no cascade)
	links := []model.Link{
		{Source: "public.orders", Target: "public.users", Cascade: true},
		{Source: "public.order_items", Target: "public.orders", Cascade: true},
		{Source: "public.order_items", Target: "public.products", Cascade: false},
	}
	if d := cascadeDepth(links); d != 2 {
		t.Errorf("cascadeDepth = %d, want 2", d)
	}
}

func TestCascadeDepthHandlesCycle(t *testing.T) {
	links := []model.Link{
		{Source: "a", Target: "b", Cascade: true},
		{Source: "b", Target: "a", Cascade: true},
	}
	// Must terminate; depth is finite.
	_ = cascadeDepth(links)
}
