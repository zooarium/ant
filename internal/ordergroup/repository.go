package ordergroup

import (
	"context"
	"fmt"

	"ant/ent"
	entorder "ant/ent/order"
	entordergroup "ant/ent/ordergroup"
	"ant/internal/order"
)

type orderGroupRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *orderGroupRepository {
	return &orderGroupRepository{client: client}
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

func (r *orderGroupRepository) List(ctx context.Context, appID, divisionID, limit, offset int, status *int8) ([]OrderGroup, error) {
	q := r.client.OrderGroup.
		Query().
		Where(entordergroup.AppID(appID), entordergroup.DivisionID(divisionID))
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

// withOrders eager-loads a group's orders (ascending) with their snapshotted
// products, so hydrateGroup can compute per-order and group totals.
func withOrders(oq *ent.OrderQuery) {
	oq.Order(ent.Asc(entorder.FieldID)).WithProducts()
}

// hydrateGroup maps a fully-loaded group entity (with the Orders edge and each
// order's Products edge) into the domain model, building the order summaries
// and the combined total. Shared by GetByID, GetByToken and ListByDevice so the
// tab assembly lives in one place.
func (r *orderGroupRepository) hydrateGroup(e *ent.OrderGroup) OrderGroup {
	g := r.mapToModel(e)
	g.Orders = make([]OrderSummary, len(e.Edges.Orders))
	for i, o := range e.Edges.Orders {
		items := make([]order.OrderItem, len(o.Edges.Products))
		var ot float64
		for j, op := range o.Edges.Products {
			items[j] = order.MapItem(op)
			ot += items[j].LineTotal
		}
		g.Orders[i] = OrderSummary{
			ID:           o.ID,
			CustomerName: o.CustomerName,
			Status:       o.Status,
			OrderedAt:    o.OrderedAt,
			TaxPercent:   o.TaxPercent,
			Total:        ot,
			Products:     items,
		}
		g.Total += ot
		g.TaxTotal += ot * o.TaxPercent / 100
	}
	g.OrdersCount = len(g.Orders)
	g.GrandTotal = g.Total + g.TaxTotal
	return g
}

func (r *orderGroupRepository) GetByID(ctx context.Context, appID, divisionID, id int) (OrderGroup, error) {
	e, err := r.client.OrderGroup.
		Query().
		Where(entordergroup.ID(id), entordergroup.AppID(appID), entordergroup.DivisionID(divisionID)).
		WithOrders(withOrders).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return OrderGroup{}, ErrOrderGroupNotFound
		}
		return OrderGroup{}, fmt.Errorf("get order group by id: %w", err)
	}
	return r.hydrateGroup(e), nil
}

// GetByToken loads a group (with its orders) by its shareable token within the
// tenant. Used by the public order-intake page so a family member who has the
// token can view the whole tab.
func (r *orderGroupRepository) GetByToken(ctx context.Context, appID, divisionID int, token string) (OrderGroup, error) {
	e, err := r.client.OrderGroup.
		Query().
		Where(entordergroup.AppID(appID), entordergroup.DivisionID(divisionID), entordergroup.Token(token)).
		WithOrders(withOrders).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return OrderGroup{}, ErrOrderGroupNotFound
		}
		return OrderGroup{}, fmt.Errorf("get order group by token: %w", err)
	}
	return r.hydrateGroup(e), nil
}

// ListByDevice returns the tabs (groups, newest first) that contain at least
// one order placed by the given device within the tenant scope, each hydrated
// with all of its orders and the combined total. Backs the public order history
// view: a returning customer sees their past tabs with everything on them.
func (r *orderGroupRepository) ListByDevice(ctx context.Context, appID, divisionID int, deviceID string, limit, offset int) ([]OrderGroup, error) {
	es, err := r.client.OrderGroup.
		Query().
		Where(
			entordergroup.AppID(appID),
			entordergroup.DivisionID(divisionID),
			entordergroup.HasOrdersWith(entorder.DeviceID(deviceID)),
		).
		WithOrders(withOrders).
		Order(ent.Desc(entordergroup.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list order groups by device: %w", err)
	}
	items := make([]OrderGroup, len(es))
	for i, e := range es {
		items[i] = r.hydrateGroup(e)
	}
	return items, nil
}

func (r *orderGroupRepository) Update(ctx context.Context, appID, divisionID, id int, label string) (OrderGroup, error) {
	count, err := r.client.OrderGroup.
		Update().
		Where(entordergroup.ID(id), entordergroup.AppID(appID), entordergroup.DivisionID(divisionID)).
		SetLabel(label).
		Save(ctx)
	if err != nil {
		return OrderGroup{}, fmt.Errorf("update order group: %w", err)
	}
	if count == 0 {
		return OrderGroup{}, ErrOrderGroupNotFound
	}
	return r.GetByID(ctx, appID, divisionID, id)
}

func (r *orderGroupRepository) UpdateStatus(ctx context.Context, appID, divisionID, id int, status int8) (OrderGroup, error) {
	count, err := r.client.OrderGroup.
		Update().
		Where(entordergroup.ID(id), entordergroup.AppID(appID), entordergroup.DivisionID(divisionID)).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return OrderGroup{}, fmt.Errorf("update order group status: %w", err)
	}
	if count == 0 {
		return OrderGroup{}, ErrOrderGroupNotFound
	}
	return r.GetByID(ctx, appID, divisionID, id)
}

// Delete removes the group. Because every order must belong to a group, a
// group that still has orders cannot be deleted; reassign or remove its orders
// first. Guarded in a transaction to avoid a delete racing a concurrent order
// attach.
func (r *orderGroupRepository) Delete(ctx context.Context, appID, divisionID, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.OrderGroup.
		Query().
		Where(entordergroup.ID(id), entordergroup.AppID(appID), entordergroup.DivisionID(divisionID)).
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
		Where(entordergroup.ID(id), entordergroup.AppID(appID), entordergroup.DivisionID(divisionID)).
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
