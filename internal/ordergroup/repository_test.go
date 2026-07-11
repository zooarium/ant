package ordergroup

import (
	"testing"

	"ant/ent"
)

// Tax is computed per member order so mixed rates within one group stay
// correct (e.g. old orders at 0% after a mid-tab rate change).
func TestHydrateGroupTotals(t *testing.T) {
	r := &orderGroupRepository{}
	e := &ent.OrderGroup{ID: 1}
	e.Edges.Orders = []*ent.Order{
		{ID: 1, TaxPercent: 10, Edges: ent.OrderEdges{Products: []*ent.OrderProduct{
			{Price: 100, Quantity: 2}, // 200
		}}},
		{ID: 2, TaxPercent: 0, Edges: ent.OrderEdges{Products: []*ent.OrderProduct{
			{Price: 50, Quantity: 1}, // 50
		}}},
	}

	g := r.hydrateGroup(e)

	if g.Total != 250 {
		t.Fatalf("Total = %v, want 250", g.Total)
	}
	if g.TaxTotal != 20 {
		t.Fatalf("TaxTotal = %v, want 20", g.TaxTotal)
	}
	if g.GrandTotal != 270 {
		t.Fatalf("GrandTotal = %v, want 270", g.GrandTotal)
	}
	if g.Orders[0].TaxPercent != 10 || len(g.Orders[0].Products) != 1 {
		t.Fatalf("order summary missing tax_percent or products: %+v", g.Orders[0])
	}
}
