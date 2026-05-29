package simulate

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

// fkGraph builds a small graph with both FK columns (for insert parent-first
// ordering) and cascade links (for delete ordering):
//
//	order_items -> orders -> users
//	order_items -> products
//
// Cascade links mirror the FK columns where ON DELETE CASCADE applies.
func fkGraph() *model.GraphModel {
	return &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.users", Columns: []model.Column{
				{Name: "id", IsPK: true},
			}},
			{ID: "public.products", Columns: []model.Column{
				{Name: "id", IsPK: true},
			}},
			{ID: "public.orders", Columns: []model.Column{
				{Name: "id", IsPK: true},
				{Name: "user_id", IsFK: true, FKRef: &model.FKRef{Table: "public.users", Column: "id", OnDelete: model.OnCascade}},
			}},
			{ID: "public.order_items", Columns: []model.Column{
				{Name: "id", IsPK: true},
				{Name: "order_id", IsFK: true, FKRef: &model.FKRef{Table: "public.orders", Column: "id", OnDelete: model.OnCascade}},
				{Name: "product_id", IsFK: true, FKRef: &model.FKRef{Table: "public.products", Column: "id", OnDelete: model.OnRestrict}},
			}},
		},
		Links: []model.Link{
			{Source: "public.orders", Target: "public.users", Cascade: true, ViaColumn: "user_id"},
			{Source: "public.order_items", Target: "public.orders", Cascade: true, ViaColumn: "order_id"},
			{Source: "public.order_items", Target: "public.products", Cascade: false, ViaColumn: "product_id"},
		},
	}
}

func TestCrudFlowDelete(t *testing.T) {
	g := fkGraph()
	res := CrudFlow(g, "public.users", ActionDelete)

	if res.Operation != "delete" || res.Root != "public.users" {
		t.Fatalf("unexpected header: %+v", res)
	}
	// Root deleted first at order 0.
	if res.Steps[0].NodeID != "public.users" || res.Steps[0].Order != 0 || res.Steps[0].Action != ActionDelete {
		t.Fatalf("expected root delete first, got %+v", res.Steps[0])
	}
	// orders (depth 1) and order_items (depth 2) cascade; products does not.
	got := map[string]int{}
	for _, s := range res.Steps {
		if s.Action != ActionDelete {
			t.Fatalf("delete flow must only have delete steps, got %q", s.Action)
		}
		got[s.NodeID] = s.Order
	}
	if _, ok := got["public.orders"]; !ok {
		t.Errorf("expected orders in delete flow")
	}
	if _, ok := got["public.order_items"]; !ok {
		t.Errorf("expected order_items in delete flow")
	}
	if _, ok := got["public.products"]; ok {
		t.Errorf("products is RESTRICT, must not appear in cascade delete flow")
	}
	// Cascade ordering: orders must precede order_items.
	if got["public.orders"] >= got["public.order_items"] {
		t.Errorf("expected orders (depth 1) before order_items (depth 2): %+v", res.Steps)
	}
	// Carries Via metadata for cascaded steps.
	for _, s := range res.Steps {
		if s.NodeID == "public.orders" && (s.Via != "public.users" || s.ViaColumn != "user_id") {
			t.Errorf("orders step missing via metadata: %+v", s)
		}
	}
}

func TestCrudFlowInsertParentFirst(t *testing.T) {
	g := fkGraph()
	res := CrudFlow(g, "public.order_items", ActionInsert)

	if res.Operation != "insert" || res.Root != "public.order_items" {
		t.Fatalf("unexpected header: %+v", res)
	}
	// Final step is the root insert.
	last := res.Steps[len(res.Steps)-1]
	if last.NodeID != "public.order_items" || last.Action != ActionInsert {
		t.Fatalf("expected root insert last, got %+v", last)
	}

	pos := map[string]int{}
	for i, s := range res.Steps {
		if s.NodeID != "public.order_items" && s.Action != ActionCheck {
			t.Fatalf("parent steps must be check, got %q for %s", s.Action, s.NodeID)
		}
		if s.Order != i {
			t.Errorf("step order should match index: %+v", s)
		}
		pos[s.NodeID] = i
	}

	// All transitive parents present: users, products, orders.
	for _, want := range []string{"public.users", "public.products", "public.orders"} {
		if _, ok := pos[want]; !ok {
			t.Errorf("expected %s as a check step, missing from %+v", want, res.Steps)
		}
	}
	// Dependency order: users must be checked before orders (orders -> users),
	// and orders before the root order_items insert.
	if pos["public.users"] >= pos["public.orders"] {
		t.Errorf("expected users checked before orders: %+v", res.Steps)
	}
	if pos["public.orders"] >= pos["public.order_items"] {
		t.Errorf("expected orders checked before root insert: %+v", res.Steps)
	}
	if pos["public.products"] >= pos["public.order_items"] {
		t.Errorf("expected products checked before root insert: %+v", res.Steps)
	}
}

func TestCrudFlowInsertNoParents(t *testing.T) {
	g := fkGraph()
	res := CrudFlow(g, "public.users", ActionInsert)
	if len(res.Steps) != 1 {
		t.Fatalf("users has no FK parents, expected 1 step, got %+v", res.Steps)
	}
	if res.Steps[0].NodeID != "public.users" || res.Steps[0].Action != ActionInsert || res.Steps[0].Order != 0 {
		t.Errorf("unexpected single insert step: %+v", res.Steps[0])
	}
}

func TestCrudFlowUpdate(t *testing.T) {
	g := fkGraph()
	res := CrudFlow(g, "public.orders", ActionUpdate)
	if len(res.Steps) != 1 {
		t.Fatalf("update must be a single step, got %+v", res.Steps)
	}
	s := res.Steps[0]
	if s.NodeID != "public.orders" || s.Action != ActionUpdate || s.Order != 0 {
		t.Errorf("unexpected update step: %+v", s)
	}
}

func TestCrudFlowInsertCycleGuard(t *testing.T) {
	// employees.manager_id -> employees (self ref) and a -> b -> a cycle.
	g := &model.GraphModel{
		Nodes: []model.Node{
			{ID: "public.a", Columns: []model.Column{
				{Name: "b_id", IsFK: true, FKRef: &model.FKRef{Table: "public.b"}},
			}},
			{ID: "public.b", Columns: []model.Column{
				{Name: "a_id", IsFK: true, FKRef: &model.FKRef{Table: "public.a"}},
			}},
		},
	}
	// Must terminate and not duplicate nodes.
	res := CrudFlow(g, "public.a", ActionInsert)
	seen := map[string]bool{}
	for _, s := range res.Steps {
		if seen[s.NodeID] {
			t.Fatalf("node %s appears more than once: %+v", s.NodeID, res.Steps)
		}
		seen[s.NodeID] = true
	}
	last := res.Steps[len(res.Steps)-1]
	if last.NodeID != "public.a" || last.Action != ActionInsert {
		t.Errorf("expected root a insert last, got %+v", last)
	}
}
