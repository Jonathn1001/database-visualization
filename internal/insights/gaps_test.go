package insights

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

const uuidType = "uuid (uuid)"

func gapsGraph() *model.GraphModel {
	return &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.users", Schema: "public", Label: "users", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "email", Type: "string (varchar(255))"},
				}},
			{ID: "public.team", Schema: "public", Label: "team", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
				}},
			{ID: "public.audit_logs", Schema: "public", Label: "audit_logs", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: uuidType},         // plural match -> users
					{Name: "team_id", Type: uuidType},         // exact match -> team
					{Name: "session_id", Type: uuidType},      // no such table
					{Name: "batch_id", Type: "bigint (int8)"}, // type mismatch vs nothing
				}},
			{ID: "public.orders", Schema: "public", Label: "orders", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: uuidType, IsFK: true,
						FKRef: &model.FKRef{Table: "public.users", Column: "id"}}, // real FK -> skip
				}},
			{ID: "public.legacy", Schema: "public", Label: "legacy", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "user_id", Type: "bigint (int8)"}, // type mismatch vs users PK -> skip
				}},
		},
	}
}

func TestRelationshipGaps(t *testing.T) {
	findings, links := RelationshipGaps(gapsGraph())

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(findings), findings)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 implied links, got %d: %+v", len(links), links)
	}

	byColumn := map[string]Finding{}
	for _, f := range findings {
		byColumn[f.Meta["column"].(string)] = f
	}

	teamGap, ok := byColumn["team_id"]
	if !ok {
		t.Fatal("expected a gap finding for audit_logs.team_id")
	}
	if teamGap.Severity != SeverityWarn {
		t.Errorf("exact-name match severity = %s, want warn", teamGap.Severity)
	}
	if teamGap.Meta["targetNodeId"] != "public.team" || teamGap.Meta["confidence"] != 0.9 {
		t.Errorf("team_id meta = %+v", teamGap.Meta)
	}
	if teamGap.Category != CategoryGaps || teamGap.NodeID != "public.audit_logs" {
		t.Errorf("team_id finding = %+v", teamGap)
	}

	userGap, ok := byColumn["user_id"]
	if !ok {
		t.Fatal("expected a gap finding for audit_logs.user_id")
	}
	if userGap.Severity != SeverityInfo || userGap.Meta["confidence"] != 0.75 {
		t.Errorf("plural match = %+v", userGap)
	}
	if userGap.Meta["targetNodeId"] != "public.users" {
		t.Errorf("user_id target = %v, want public.users", userGap.Meta["targetNodeId"])
	}

	for _, l := range links {
		if !l.Inferred || l.Confidence == 0 || l.Source != "public.audit_logs" {
			t.Errorf("implied link not inferred/confident/sourced correctly: %+v", l)
		}
		if l.Type != model.Link1ToN {
			t.Errorf("implied link type = %s, want 1:N", l.Type)
		}
	}

	// Findings and links must correspond positionally: links[i] is the
	// inferred edge for findings[i], not merely some matching edge elsewhere
	// in the slice.
	for i := range findings {
		lk, fd := links[i], findings[i]
		if lk.Target != fd.Meta["targetNodeId"] {
			t.Errorf("index %d: link target %q != finding targetNodeId %v", i, lk.Target, fd.Meta["targetNodeId"])
		}
		if lk.Confidence != fd.Meta["confidence"] {
			t.Errorf("index %d: link confidence %v != finding confidence %v", i, lk.Confidence, fd.Meta["confidence"])
		}
		if lk.ViaColumn != fd.Meta["column"] {
			t.Errorf("index %d: link viaColumn %q != finding column %v", i, lk.ViaColumn, fd.Meta["column"])
		}
	}
}

func TestRelationshipGapsEmptyGraph(t *testing.T) {
	findings, links := RelationshipGaps(&model.GraphModel{})
	if len(findings) != 0 || len(links) != 0 {
		t.Errorf("expected nothing, got %d findings, %d links", len(findings), len(links))
	}
}

// TestRelationshipGapsSelfReferenceSkip covers a table whose own label
// matches a column's base name (e.g. public.team.team_id): the "target" is
// the source node itself, so no gap should be reported.
func TestRelationshipGapsSelfReferenceSkip(t *testing.T) {
	g := &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.team", Schema: "public", Label: "team", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "team_id", Type: uuidType}, // self-referential name+type match — must be skipped
				}},
		},
	}

	findings, links := RelationshipGaps(g)
	if len(findings) != 0 || len(links) != 0 {
		t.Errorf("expected no findings for a self-referential column match, got %d findings, %d links: %+v",
			len(findings), len(links), findings)
	}
}

// TestRelationshipGapsConfidenceTieBreak covers two same-schema candidate
// targets tied at plural-match confidence (0.75): public.parts and
// public.partes both satisfy public.orders.part_id equally, so exactly one
// finding must be produced, targeting the lexicographically smaller node ID
// ("public.partes" < "public.parts", since 'e' < 's').
func TestRelationshipGapsConfidenceTieBreak(t *testing.T) {
	g := &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.parts", Schema: "public", Label: "parts", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
				}},
			{ID: "public.partes", Schema: "public", Label: "partes", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
				}},
			{ID: "public.orders", Schema: "public", Label: "orders", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "part_id", Type: uuidType}, // matches two equally-confident plural targets
				}},
		},
	}

	findings, links := RelationshipGaps(g)
	if len(findings) != 1 || len(links) != 1 {
		t.Fatalf("expected exactly 1 finding for a tied-confidence column, got %d findings, %d links: %+v",
			len(findings), len(links), findings)
	}
	if findings[0].Meta["confidence"] != 0.75 {
		t.Errorf("tie-break confidence = %v, want 0.75", findings[0].Meta["confidence"])
	}
	if findings[0].Meta["targetNodeId"] != "public.partes" {
		t.Errorf("tie-break target = %v, want public.partes (lexicographically smaller)", findings[0].Meta["targetNodeId"])
	}
	if links[0].Target != "public.partes" {
		t.Errorf("tie-break link target = %s, want public.partes", links[0].Target)
	}
}

// TestRelationshipGapsCrossSchemaExclusion covers a candidate target that
// matches a column's base name and type but lives in a different schema:
// audit.teams must not match public.events.team_id when no public.team(s)
// table exists.
func TestRelationshipGapsCrossSchemaExclusion(t *testing.T) {
	g := &model.GraphModel{
		Nodes: []model.Node{
			{ID: "audit.teams", Schema: "audit", Label: "teams", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
				}},
			{ID: "public.events", Schema: "public", Label: "events", Kind: model.KindTable,
				Columns: []model.Column{
					{Name: "id", Type: uuidType, IsPK: true},
					{Name: "team_id", Type: uuidType}, // matches audit.teams' shape but not its schema
				}},
		},
	}

	findings, links := RelationshipGaps(g)
	if len(findings) != 0 || len(links) != 0 {
		t.Errorf("expected no cross-schema findings, got %d findings, %d links: %+v",
			len(findings), len(links), findings)
	}
}
