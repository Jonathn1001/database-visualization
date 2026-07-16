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
}

func TestRelationshipGapsEmptyGraph(t *testing.T) {
	findings, links := RelationshipGaps(&model.GraphModel{})
	if len(findings) != 0 || len(links) != 0 {
		t.Errorf("expected nothing, got %d findings, %d links", len(findings), len(links))
	}
}
