package ordergroup

import (
	"context"
	"fmt"

	"ant/ent"
	entorder "ant/ent/order"
	entordergroup "ant/ent/ordergroup"
)

type orderGroupRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *orderGroupRepository {
	return &orderGroupRepository{client: client}
}

// orderProductTotal computes the line total of one snapshotted order item:
// (base price + sum of chosen option deltas) * quantity.
func orderProductTotal(op *ent.OrderProduct) float64 {
	unit := op.Price
	for _, a := range op.Attributes {
		unit += a.PriceDelta
	}
	return unit * float64(op.Quantity)
}

func (r *orderGroupRepository) Create(ctx context.Context, item OrderGroup) (OrderGroup, error) {
	e, err := r.client.OrderGroup.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetDivisionID(item.DivisionID).
		SetToken(item.Token).
		SetLabel(item.Label).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return OrderGroup{}, fmt.Errorf("create order group: %w", err)
	}
	return r.mapToModel(e), nil
}

func (r *orderGroupRepository) List(ctx context.Context, appID, limit, offset int, status *int8) ([]OrderGroup, error) {
	q := r.client.OrderGroup.
		Query().
		Where(entordergroup.AppID(appID))
	if status != nil {
		q = q.Where(entordergroup.Status(*status))
	}
	es, err := q.
		Order(ent.Asc(entordergroup.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list order groups: %w", err)
	}
	if len(es) == 0 {
		return []OrderGroup{}, nil
	}

	// Order counts for the page in one grouped query.
	ids := make([]int, len(es))
	for i, e := range es {
		ids[i] = e.ID
	}
	var rows []struct {
		GroupID int `json:"group_id"`
		Count   int `json:"count"`
	}
	if err := r.client.Order.
		Query().
		Where(entorder.GroupIDIn(ids...)).
		GroupBy(entorder.FieldGroupID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("count group orders: %w", err)
	}
	counts := make(map[int]int, len(rows))
	for _, row := range rows {
		counts[row.GroupID] = row.Count
	}

	items := make([]OrderGroup, len(es))
	for i, e := range es {
		g := r.mapToModel(e)
		g.OrdersCount = counts[e.ID]
		items[i] = g
	}
	return items, nil
}

func (r *orderGroupRepository) GetByID(ctx context.Context, appID, id int) (OrderGroup, error) {
	e, err := r.client.OrderGroup.
		Query().
		Where(entordergroup.ID(id), entordergroup.AppID(appID)).
		WithOrders(func(oq *ent.OrderQuery) {
			oq.Order(ent.Asc(entorder.FieldID)).
				WithProducts()
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return OrderGroup{}, ErrOrderGroupNotFound
		}
		return OrderGroup{}, fmt.Errorf("get order group by id: %w", err)
	}
	g := r.mapToModel(e)
	g.Orders = make([]OrderSummary, len(e.Edges.Orders))
	var total float64
	for i, o := range e.Edges.Orders {
		var ot float64
		for _, op := range o.Edges.Products {
			ot += orderProductTotal(op)
		}
		g.Orders[i] = OrderSummary{
			ID:           o.ID,
			CustomerName: o.CustomerName,
			Status:       o.Status,
			OrderedAt:    o.OrderedAt,
			Total:        ot,
		}
		total += ot
	}
	g.OrdersCount = len(g.Orders)
	g.Total = total
	return g, nil
}

func (r *orderGroupRepository) Update(ctx context.Context, appID, id int, label string) (OrderGroup, error) {
	count, err := r.client.OrderGroup.
		Update().
		Where(entordergroup.ID(id), entordergroup.AppID(appID)).
		SetLabel(label).
		Save(ctx)
	if err != nil {
		return OrderGroup{}, fmt.Errorf("update order group: %w", err)
	}
	if count == 0 {
		return OrderGroup{}, ErrOrderGroupNotFound
	}
	return r.GetByID(ctx, appID, id)
}

func (r *orderGroupRepository) UpdateStatus(ctx context.Context, appID, id int, status int8) (OrderGroup, error) {
	count, err := r.client.OrderGroup.
		Update().
		Where(entordergroup.ID(id), entordergroup.AppID(appID)).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return OrderGroup{}, fmt.Errorf("update order group status: %w", err)
	}
	if count == 0 {
		return OrderGroup{}, ErrOrderGroupNotFound
	}
	return r.GetByID(ctx, appID, id)
}

// Delete removes the group. Because every order must belong to a group, a
// group that still has orders cannot be deleted; reassign or remove its orders
// first. Guarded in a transaction to avoid a delete racing a concurrent order
// attach.
func (r *orderGroupRepository) Delete(ctx context.Context, appID, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.OrderGroup.
		Query().
		Where(entordergroup.ID(id), entordergroup.AppID(appID)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check order group exists: %w", err))
	}
	if !exists {
		return rollback(tx, ErrOrderGroupNotFound)
	}
	used, err := tx.Order.
		Query().
		Where(entorder.GroupID(id)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check order group in use: %w", err))
	}
	if used {
		return rollback(tx, ErrOrderGroupInUse)
	}
	if _, err := tx.OrderGroup.
		Delete().
		Where(entordergroup.ID(id), entordergroup.AppID(appID)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete order group: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return fmt.Errorf("%w: rollback: %v", err, rerr)
	}
	return err
}

func (r *orderGroupRepository) mapToModel(e *ent.OrderGroup) OrderGroup {
	return OrderGroup{
		ID:         e.ID,
		AppID:      e.AppID,
		UserID:     e.UserID,
		DivisionID: e.DivisionID,
		Token:      e.Token,
		Label:      e.Label,
		Status:     e.Status,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}
