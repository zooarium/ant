package order

import (
	"context"
	"fmt"
	"time"

	"ant/ent"
	entorder "ant/ent/order"
	entordergroup "ant/ent/ordergroup"
	"ant/ent/orderproduct"
	entproduct "ant/ent/product"
	"ant/ent/schema"

	"github.com/google/uuid"
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

		// The option must be in the product's allowed subset for this
		// attribute; its price delta is read from there.
		var delta float64
		found := false
		for _, po := range pa.Options {
			if po.OptionID == ia.OptionID {
				delta = po.PriceDelta
				found = true
				break
			}
		}
		if !found {
			return nil, nil, ErrInvalidOption
		}

		// Resolve the display value from the live catalogue.
		var value string
		for _, o := range attr.Edges.Options {
			if o.ID == ia.OptionID {
				value = o.Value
				break
			}
		}

		snapshot = append(snapshot, schema.OrderItemAttribute{
			AttributeID:   attr.ID,
			AttributeName: attr.Name,
			OptionID:      ia.OptionID,
			OptionValue:   value,
			PriceDelta:    delta,
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

// createItem writes one order item and returns its line total
// ((price + option deltas) * quantity) so callers can accumulate the order total.
func createItem(ctx context.Context, tx *ent.Tx, appID, orderID, productID, quantity int, attributes []OrderItemAttributeRequest) (float64, error) {
	p, snapshot, err := buildSnapshot(ctx, tx, appID, productID, attributes)
	if err != nil {
		return 0, err
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
		return 0, fmt.Errorf("create order item: %w", err)
	}
	unit := p.Price
	for _, a := range snapshot {
		unit += a.PriceDelta
	}
	return unit * float64(quantity), nil
}

// sumOrderItems recomputes an order's total from its persisted items within the
// surrounding transaction. Used after a sync where kept items retain their own
// snapshots, so accumulation from the request alone is insufficient.
func sumOrderItems(ctx context.Context, tx *ent.Tx, orderID int) (float64, error) {
	items, err := tx.OrderProduct.
		Query().
		Where(orderproduct.OrderID(orderID)).
		Select(orderproduct.FieldPrice, orderproduct.FieldQuantity, orderproduct.FieldAttributes).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("sum order items: %w", err)
	}
	var total float64
	for _, it := range items {
		total += mapItem(it).LineTotal
	}
	return total, nil
}

// verifyGroup ensures the group exists and belongs to the app. App scoping is
// not enforced by the FK, so it must be checked explicitly.
func verifyGroup(ctx context.Context, tx *ent.Tx, appID, groupID int) error {
	ok, err := tx.OrderGroup.
		Query().
		Where(entordergroup.ID(groupID), entordergroup.AppID(appID)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("verify order group: %w", err)
	}
	if !ok {
		return ErrOrderGroupInvalid
	}
	return nil
}

func (r *orderRepository) Create(ctx context.Context, item Order, items []OrderItemRequest, groupID *int, groupLabel string) (Order, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	id, err := buildOrderInTx(ctx, tx, item, items, groupID, groupLabel)
	if err != nil {
		return Order{}, rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, item.AppID, item.UserID, id)
}

// CreatePublic creates an order on behalf of a public (guest) caller, enforcing
// a per-identity volume cap inside the same transaction as the insert so the
// check and write cannot race: the count and the create see one consistent
// snapshot. The cap counts orders within window for the same tenant scope keyed
// on device id when supplied, else on client IP. maxOrders <= 0 disables it.
func (r *orderRepository) CreatePublic(ctx context.Context, item Order, items []OrderItemRequest, groupID *int, groupLabel string, maxOrders int, window time.Duration) (Order, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	if maxOrders > 0 {
		q := tx.Order.
			Query().
			Where(
				entorder.AppID(item.AppID),
				entorder.DivisionID(item.DivisionID),
				entorder.CreatedAtGTE(time.Now().Add(-window)),
			)
		if item.DeviceID != "" {
			q = q.Where(entorder.DeviceID(item.DeviceID))
		} else {
			q = q.Where(entorder.IPAddress(item.IPAddress))
		}
		n, err := q.Count(ctx)
		if err != nil {
			return Order{}, rollback(tx, fmt.Errorf("count public orders: %w", err))
		}
		if n >= maxOrders {
			return Order{}, rollback(tx, ErrPublicOrderLimit)
		}
	}
	id, err := buildOrderInTx(ctx, tx, item, items, groupID, groupLabel)
	if err != nil {
		return Order{}, rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, item.AppID, item.UserID, id)
}

// buildOrderInTx resolves/mints the group, inserts the order and its items, and
// sets the order total — all on the supplied transaction. It returns the new
// order id; the caller owns commit/rollback. Shared by Create and CreatePublic.
func buildOrderInTx(ctx context.Context, tx *ent.Tx, item Order, items []OrderItemRequest, groupID *int, groupLabel string) (int, error) {
	// Resolve the group: attach to the supplied one, or mint a fresh group in
	// the same transaction so a single order-create call both places the order
	// and opens its tab. Every order belongs to exactly one group.
	if groupID != nil {
		if err := verifyGroup(ctx, tx, item.AppID, *groupID); err != nil {
			return 0, err
		}
		item.GroupID = *groupID
	} else {
		g, err := tx.OrderGroup.
			Create().
			SetAppID(item.AppID).
			SetUserID(item.UserID).
			SetDivisionID(item.DivisionID).
			SetToken(uuid.NewString()).
			SetLabel(groupLabel).
			SetStatus(1).
			Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("create order group: %w", err)
		}
		item.GroupID = g.ID
	}
	create := tx.Order.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetDivisionID(item.DivisionID).
		SetCustomerName(item.CustomerName).
		SetCustomerContact(item.CustomerContact).
		SetStatus(item.Status).
		SetTaxPercent(item.TaxPercent).
		SetGroupID(item.GroupID)
	if item.IPAddress != "" {
		create.SetIPAddress(item.IPAddress)
	}
	if item.DeviceID != "" {
		create.SetDeviceID(item.DeviceID)
	}
	if !item.OrderedAt.IsZero() {
		create.SetOrderedAt(item.OrderedAt)
	}
	e, err := create.Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("create order: %w", err)
	}
	var total float64
	for _, req := range items {
		lt, err := createItem(ctx, tx, item.AppID, e.ID, req.ProductID, req.Quantity, req.Attributes)
		if err != nil {
			return 0, err
		}
		total += lt
	}
	if _, err := tx.Order.UpdateOneID(e.ID).SetTotal(total).Save(ctx); err != nil {
		return 0, fmt.Errorf("set order total: %w", err)
	}
	return e.ID, nil
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
		WithGroup().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("get order by id: %w", err)
	}
	o := r.mapToModel(e)
	if e.Edges.Group != nil {
		o.GroupToken = e.Edges.Group.Token
	}
	o.Products = make([]OrderItem, len(e.Edges.Products))
	var total float64
	for i, op := range e.Edges.Products {
		o.Products[i] = mapItem(op)
		total += o.Products[i].LineTotal
	}
	o.ProductsCount = len(o.Products)
	o.Total = total
	return o, nil
}

// Update atomically updates the order's customer details and syncs its items
// in one transaction: payload items with an id update the existing item's
// quantity (the snapshot is immutable), ones without an id are added from the
// live catalogue, and existing items absent from the payload are deleted.
func (r *orderRepository) Update(ctx context.Context, appID, userID, id int, item Order, items []SyncOrderItemRequest, taxPercent *float64) (Order, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	upd := tx.Order.
		Update().
		Where(entorder.ID(id), entorder.AppID(appID)).
		SetCustomerName(item.CustomerName).
		SetCustomerContact(item.CustomerContact)
	// tax_percent is preserved when the payload omits it.
	if taxPercent != nil {
		upd.SetTaxPercent(*taxPercent)
	}
	if !item.OrderedAt.IsZero() {
		upd.SetOrderedAt(item.OrderedAt)
	}
	count, err := upd.Save(ctx)
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
			if _, err := createItem(ctx, tx, appID, id, p.ProductID, p.Quantity, p.Attributes); err != nil {
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

	total, err := sumOrderItems(ctx, tx, id)
	if err != nil {
		return Order{}, rollback(tx, err)
	}
	if _, err := tx.Order.UpdateOneID(id).SetTotal(total).Save(ctx); err != nil {
		return Order{}, rollback(tx, fmt.Errorf("set order total: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, appID, userID, id)
}

// SetGroup moves the order to a different group. The target group must belong
// to the same app. Orders always belong to a group, so there is no detach.
func (r *orderRepository) SetGroup(ctx context.Context, appID, userID, id, groupID int) (Order, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.Order.
		Query().
		Where(entorder.ID(id), entorder.AppID(appID)).
		Exist(ctx)
	if err != nil {
		return Order{}, rollback(tx, fmt.Errorf("check order exists: %w", err))
	}
	if !exists {
		return Order{}, rollback(tx, ErrOrderNotFound)
	}
	if err := verifyGroup(ctx, tx, appID, groupID); err != nil {
		return Order{}, rollback(tx, err)
	}
	if _, err := tx.Order.
		Update().
		Where(entorder.ID(id), entorder.AppID(appID)).
		SetGroupID(groupID).
		Save(ctx); err != nil {
		return Order{}, rollback(tx, fmt.Errorf("set order group: %w", err))
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
	groupID, err := tx.Order.
		Query().
		Where(entorder.ID(id), entorder.AppID(appID)).
		Select(entorder.FieldGroupID).
		Int(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(tx, ErrOrderNotFound)
		}
		return rollback(tx, fmt.Errorf("get order group: %w", err))
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
	// An order group only has meaning as a tab over its orders. Once the last
	// order leaves the group, delete the now-empty group in the same tx. The
	// re-check inside the tx keeps this race-free against concurrent inserts.
	remaining, err := tx.Order.
		Query().
		Where(entorder.GroupID(groupID)).
		Count(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("count remaining group orders: %w", err))
	}
	if remaining == 0 {
		if _, err := tx.OrderGroup.
			Delete().
			Where(entordergroup.ID(groupID), entordergroup.AppID(appID)).
			Exec(ctx); err != nil {
			return rollback(tx, fmt.Errorf("delete empty order group: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *orderRepository) mapToModel(e *ent.Order) Order {
	return Order{
		ID:           e.ID,
		AppID:        e.AppID,
		UserID:       e.UserID,
		DivisionID:   e.DivisionID,
		GroupID:      e.GroupID,
		CustomerName: e.CustomerName,
		// GroupID is non-null by schema; e.GroupID is int.
		CustomerContact: e.CustomerContact,
		Status:          e.Status,
		TaxPercent:      e.TaxPercent,
		Total:           e.Total,
		IPAddress:       e.IPAddress,
		DeviceID:        e.DeviceID,
		OrderedAt:       e.OrderedAt,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

func mapItem(e *ent.OrderProduct) OrderItem {
	attrs := make([]OrderItemAttribute, len(e.Attributes))
	unit := e.Price
	for i, a := range e.Attributes {
		attrs[i] = OrderItemAttribute{
			AttributeID:   a.AttributeID,
			AttributeName: a.AttributeName,
			OptionID:      a.OptionID,
			OptionValue:   a.OptionValue,
			PriceDelta:    a.PriceDelta,
		}
		unit += a.PriceDelta
	}
	return OrderItem{
		ID:          e.ID,
		ProductID:   e.ProductID,
		ProductName: e.ProductName,
		Price:       e.Price,
		Quantity:    e.Quantity,
		Attributes:  attrs,
		LineTotal:   unit * float64(e.Quantity),
	}
}

func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return fmt.Errorf("%w: rollback: %v", err, rerr)
	}
	return err
}
