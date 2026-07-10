package category

import (
	"context"
	"errors"
	"testing"

	"ant/ent/enttest"

	_ "github.com/mattn/go-sqlite3"
)

// TestReorder covers the one non-trivial path added for custom ordering: the
// atomic bulk ord update, its tenant-scope re-check inside the transaction,
// and the ord-first list order.
func TestReorder(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	a := client.Category.Create().SetAppID(1).SetDivisionID(1).SetName("A").SetPath("/1/").SaveX(ctx)
	b := client.Category.Create().SetAppID(1).SetDivisionID(1).SetName("B").SetPath("/2/").SaveX(ctx)
	other := client.Category.Create().SetAppID(2).SetDivisionID(1).SetName("X").SetPath("/3/").SaveX(ctx)

	repo := NewRepository(client)
	svc := NewService(repo)

	// B before A.
	if err := svc.Reorder(ctx, 1, 1, ReorderRequest{Items: []ReorderItem{{ID: a.ID, Ord: 2}, {ID: b.ID, Ord: 1}}}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	rows, err := svc.List(ctx, 1, 1, nil, nil, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != b.ID || rows[1].ID != a.ID {
		t.Fatalf("expected order [B A], got %+v", rows)
	}

	// Cross-tenant id rejected wholesale, nothing persisted.
	err = svc.Reorder(ctx, 1, 1, ReorderRequest{Items: []ReorderItem{{ID: a.ID, Ord: 9}, {ID: other.ID, Ord: 1}}})
	if !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got %v", err)
	}
	got, _ := svc.GetByID(ctx, 1, 1, a.ID)
	if got.Ord != 2 {
		t.Fatalf("cross-tenant reorder leaked: ord=%d", got.Ord)
	}

	// Duplicate ids rejected before touching the DB.
	if err := svc.Reorder(ctx, 1, 1, ReorderRequest{Items: []ReorderItem{{ID: a.ID, Ord: 1}, {ID: a.ID, Ord: 2}}}); !errors.Is(err, ErrDuplicateReorder) {
		t.Fatalf("expected ErrDuplicateReorder, got %v", err)
	}
}
