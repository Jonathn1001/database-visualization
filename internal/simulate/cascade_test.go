package simulate

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

func TestCascade(t *testing.T) {
	g := &model.GraphModel{
		Links: []model.Link{
			{Source: "public.orders", Target: "public.users", Cascade: true, ViaColumn: "user_id"},
			{Source: "public.addresses", Target: "public.users", Cascade: true, ViaColumn: "user_id"},
			{Source: "public.reviews", Target: "public.users", Cascade: true, ViaColumn: "user_id"},
			{Source: "public.order_items", Target: "public.orders", Cascade: true, ViaColumn: "order_id"},
			{Source: "public.payments", Target: "public.orders", Cascade: true, ViaColumn: "order_id"},
			{Source: "public.order_items", Target: "public.products", Cascade: false, ViaColumn: "product_id"},
		},
	}
	res := Cascade(g, "public.users")

	got := map[string]bool{}
	for _, s := range res.Affected {
		got[s.NodeID] = true
	}
	for _, want := range []string{"public.orders", "public.addresses", "public.reviews", "public.order_items", "public.payments"} {
		if !got[want] {
			t.Errorf("expected %s in cascade, missing", want)
		}
	}
	// order_items is reached only via orders (cascade), not products (restrict).
	if len(res.Affected) != 5 {
		t.Errorf("expected 5 affected tables, got %d: %+v", len(res.Affected), res.Affected)
	}
}

func TestCascadeNoLinks(t *testing.T) {
	g := &model.GraphModel{}
	if res := Cascade(g, "x"); len(res.Affected) != 0 {
		t.Errorf("expected no affected nodes, got %+v", res.Affected)
	}
}
