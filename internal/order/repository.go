package order

import (
	"context"
	"fmt"

	"ant/ent"
	entorder "ant/ent/order"
	"ant/ent/orderproduct"
	entproduct "ant/ent/product"
	"ant/ent/schema"
)

type orderRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *orderRepository {
	return &orderRepository{client: client}
}

// buildSnapshot validates an item request against the live catalogue and
// returns the denormalized snapshot to store with the order. It must run
// inside the surrounding transaction so the checks and writes are atomic.
func buildSnapshot(ctx context.Context, tx *ent.Tx, appID, productID int, attributes []OrderItemAttributeRequest) (*ent.Product, []schema.OrderItemAttribute, error) {
	p, err := tx.Product.
		Query().
		Where(entproduct.ID(productID), entproduct.AppID(appID)).
		WithAttributes(func(paq *ent.ProductAttributeQuery) {
			paq.WithAttribute(func(aq *ent.AttributeQuery) {
				aq.WithOptions()
			})
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, ErrProductInvalid
		}
		return nil, nil, fmt.Errorf("get product for order item: %w", err)
	}
	if p.Status != 1 {
		return nil, nil, ErrProductInactive
	}

	assigned := make(map[int]*ent.ProductAttribute, len(p.Edges.Attributes))
	for _, pa := range p.Edges.Attributes {
		assigned[pa.AttributeID] = pa
	}

	chosen := make(map[int]struct{}, len(attributes))
	snapshot := make([]schema.OrderItemAttribute, 0, len(attributes))
	for _, ia := range attributes {
		if _, ok := chosen[ia.AttributeID]; ok {
			return nil, nil, ErrDuplicateItemAttribute
		}
		chosen[ia.AttributeID] = struct{}{}

		pa, ok := assigned[ia.AttributeID]
		if !ok || pa.Edges.Attribute == nil {
			return nil, nil, ErrAttributeNotAssigned
		}
		attr := pa.Edges.Attribute

		var value string
		found := false
		for _, o := range attr.Edges.Options {
			if o.ID == ia.OptionID {
				value = o.Value
				found = true
				break
			}
		}
		if !found {
			return nil, nil, ErrInvalidOption
		}

		snapshot = append(snapshot, schema.OrderItemAttribute{
			AttributeID:   attr.ID,
			AttributeName: attr.Name,
			OptionID:      ia.OptionID,
			OptionValue:   value,
		})
	}

	for _, pa := range p.Edges.Attributes {
		if pa.IsMandatory {
			if _, ok := chosen[pa.AttributeID]; !ok {
				return nil, nil, ErrMandatoryAttributeMissing
			}
		}
	}

	return p, snapshot, nil
}

func createItem(ctx context.Context, tx *ent.Tx, appID, orderID, productID, quantity int, attributes []OrderItemAttributeRequest) error {
	p, snapshot, err := buildSnapshot(ctx, tx, appID, productID, attributes)
	if err != nil {
		return err
	}
	if _, err := tx.OrderProduct.
		Create().
		SetOrderID(orderID).
		SetProductID(p.ID).
		SetProductName(p.Name).
		SetPrice(p.Price).
		SetQuantity(quantity).
		SetAttributes(snapshot).
		Save(ctx); err != nil {
		return fmt.Errorf("create order item: %w", err)
	}
	return nil
}

func (r *orderRepository) Create(ctx context.Context, item Order, items []OrderItemRequest) (Order, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	e, err := tx.Order.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetDivisionID(item.DivisionID).
		SetCustomerName(item.CustomerName).
		SetCustomerContact(item.CustomerContact).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Order{}, rollback(tx, fmt.Errorf("create order: %w", err))
	}
	for _, req := range items {
		if err := createItem(ctx, tx, item.AppID, e.ID, req.ProductID, req.Quantity, req.Attributes); err != nil {
			return Order{}, rollback(tx, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, item.AppID, item.UserID, e.ID)
}

func (r *orderRepository) List(ctx context.Context, appID, userID, limit, offset int, status *int8) ([]Order, error) {
	q := r.client.Order.
		Query().
		Where(entorder.AppID(appID))
	if status != nil {
		q = q.Where(entorder.Status(*status))
	}
	es, err := q.
		Order(ent.Asc(entorder.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	if len(es) == 0 {
		return []Order{}, nil
	}

	// Item counts for the page in one grouped query (summary listing).
	ids := make([]int, len(es))
	for i, e := range es {
		ids[i] = e.ID
	}
	var rows []struct {
		OrderID int `json:"order_id"`
		Count   int `json:"count"`
	}
	if err := r.client.OrderProduct.
		Query().
		Where(orderproduct.OrderIDIn(ids...)).
		GroupBy(orderproduct.FieldOrderID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("count order items: %w", err)
	}
	counts := make(map[int]int, len(rows))
	for _, row := range rows {
		counts[row.OrderID] = row.Count
	}

	items := make([]Order, len(es))
	for i, e := range es {
		o := r.mapToModel(e)
		o.ProductsCount = counts[e.ID]
		items[i] = o
	}
	return items, nil
}

func (r *orderRepository) GetByID(ctx context.Context, appID, userID, id int) (Order, error) {
	e, err := r.client.Order.
		Query().
		Where(entorder.ID(id), entorder.AppID(appID)).
		WithProducts(func(opq *ent.OrderProductQuery) {
			opq.Order(ent.Asc(orderproduct.FieldID))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("get order by id: %w", err)
	}
	o := r.mapToModel(e)
	o.Products = make([]OrderItem, len(e.Edges.Products))
	for i, op := range e.Edges.Products {
		o.Products[i] = mapItem(op)
	}
	o.ProductsCount = len(o.Products)
	return o, nil
}

// Update atomically updates the order's customer details and syncs its items
// in one transaction: payload items with an id update the existing item's
// quantity (the snapshot is immutable), ones without an id are added from the
// live catalogue, and existing items absent from the payload are deleted.
func (r *orderRepository) Update(ctx context.Context, appID, userID, id int, item Order, items []SyncOrderItemRequest) (Order, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	count, err := tx.Order.
		Update().
		Where(entorder.ID(id), entorder.AppID(appID)).
		SetCustomerName(item.CustomerName).
		SetCustomerContact(item.CustomerContact).
		Save(ctx)
	if err != nil {
		return Order{}, rollback(tx, fmt.Errorf("update order: %w", err))
	}
	if count == 0 {
		return Order{}, rollback(tx, ErrOrderNotFound)
	}

	existing, err := tx.OrderProduct.
		Query().
		Where(orderproduct.OrderID(id)).
		Select(orderproduct.FieldID).
		Ints(ctx)
	if err != nil {
		return Order{}, rollback(tx, fmt.Errorf("list order items: %w", err))
	}
	existingIDs := make(map[int]struct{}, len(existing))
	for _, iid := range existing {
		existingIDs[iid] = struct{}{}
	}

	keep := make(map[int]struct{}, len(items))
	for _, p := range items {
		if p.ID == 0 {
			if err := createItem(ctx, tx, appID, id, p.ProductID, p.Quantity, p.Attributes); err != nil {
				return Order{}, rollback(tx, err)
			}
			continue
		}
		if _, ok := existingIDs[p.ID]; !ok {
			return Order{}, rollback(tx, ErrOrderItemNotFound)
		}
		if _, err := tx.OrderProduct.
			Update().
			Where(orderproduct.ID(p.ID), orderproduct.OrderID(id)).
			SetQuantity(p.Quantity).
			Save(ctx); err != nil {
			return Order{}, rollback(tx, fmt.Errorf("update order item quantity: %w", err))
		}
		keep[p.ID] = struct{}{}
	}

	remove := make([]int, 0, len(existing))
	for _, iid := range existing {
		if _, ok := keep[iid]; !ok {
			remove = append(remove, iid)
		}
	}
	if len(remove) > 0 {
		if _, err := tx.OrderProduct.
			Delete().
			Where(orderproduct.IDIn(remove...), orderproduct.OrderID(id)).
			Exec(ctx); err != nil {
			return Order{}, rollback(tx, fmt.Errorf("delete order items: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, appID, userID, id)
}

func (r *orderRepository) UpdateStatus(ctx context.Context, appID, userID, id int, status int8) (Order, error) {
	count, err := r.client.Order.
		Update().
		Where(entorder.ID(id), entorder.AppID(appID)).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("update order status: %w", err)
	}
	if count == 0 {
		return Order{}, ErrOrderNotFound
	}
	return r.GetByID(ctx, appID, userID, id)
}

func (r *orderRepository) Delete(ctx context.Context, appID, userID, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.Order.
		Query().
		Where(entorder.ID(id), entorder.AppID(appID)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check order exists: %w", err))
	}
	if !exists {
		return rollback(tx, ErrOrderNotFound)
	}
	if _, err := tx.OrderProduct.
		Delete().
		Where(orderproduct.OrderID(id)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete order items: %w", err))
	}
	if _, err := tx.Order.
		Delete().
		Where(entorder.ID(id), entorder.AppID(appID)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete order: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *orderRepository) mapToModel(e *ent.Order) Order {
	return Order{
		ID:              e.ID,
		AppID:           e.AppID,
		UserID:          e.UserID,
		DivisionID:      e.DivisionID,
		CustomerName:    e.CustomerName,
		CustomerContact: e.CustomerContact,
		Status:          e.Status,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

func mapItem(e *ent.OrderProduct) OrderItem {
	attrs := make([]OrderItemAttribute, len(e.Attributes))
	for i, a := range e.Attributes {
		attrs[i] = OrderItemAttribute{
			AttributeID:   a.AttributeID,
			AttributeName: a.AttributeName,
			OptionID:      a.OptionID,
			OptionValue:   a.OptionValue,
		}
	}
	return OrderItem{
		ID:          e.ID,
		ProductID:   e.ProductID,
		ProductName: e.ProductName,
		Price:       e.Price,
		Quantity:    e.Quantity,
		Attributes:  attrs,
	}
}

func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return fmt.Errorf("%w: rollback: %v", err, rerr)
	}
	return err
}
