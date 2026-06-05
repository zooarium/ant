package order

import (
	"context"
	"fmt"

	"ant/ent"
	"ant/ent/order"
)

type orderRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *orderRepository {
	return &orderRepository{client: client}
}

func (r *orderRepository) Create(ctx context.Context, item Order) (Order, error) {
	e, err := r.client.Order.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetName(item.Name).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	return r.mapToModel(e), nil
}

func (r *orderRepository) List(ctx context.Context, appID, userID, limit, offset int) ([]Order, error) {
	es, err := r.client.Order.
		Query().
		Where(order.AppID(appID)).
		Order(ent.Asc(order.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	items := make([]Order, len(es))
	for i, e := range es {
		items[i] = r.mapToModel(e)
	}
	return items, nil
}

func (r *orderRepository) GetByID(ctx context.Context, appID, userID, id int) (Order, error) {
	e, err := r.client.Order.
		Query().
		Where(order.ID(id), order.AppID(appID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("get order by id: %w", err)
	}
	return r.mapToModel(e), nil
}

func (r *orderRepository) Update(ctx context.Context, appID, userID, id int, item Order) (Order, error) {
	count, err := r.client.Order.
		Update().
		Where(order.ID(id), order.AppID(appID)).
		SetName(item.Name).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("update order: %w", err)
	}
	if count == 0 {
		return Order{}, ErrOrderNotFound
	}
	return r.GetByID(ctx, appID, userID, id)
}

func (r *orderRepository) Delete(ctx context.Context, appID, userID, id int) error {
	count, err := r.client.Order.
		Delete().
		Where(order.ID(id), order.AppID(appID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete order: %w", err)
	}
	if count == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func (r *orderRepository) mapToModel(e *ent.Order) Order {
	return Order{
		ID:        e.ID,
		AppID:     e.AppID,
		UserID:    e.UserID,
		Name:      e.Name,
		Status:    e.Status,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
