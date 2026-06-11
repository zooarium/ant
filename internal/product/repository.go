package product

import (
	"context"
	"fmt"

	"ant/ent"
	entattribute "ant/ent/attribute"
	"ant/ent/attributeoption"
	"ant/ent/orderproduct"
	entproduct "ant/ent/product"
	"ant/ent/productattribute"
	"ant/ent/schema"
)

type productRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *productRepository {
	return &productRepository{client: client}
}

// verifyAttributes ensures every assigned attribute exists, belongs to the
// app and is active, and that every option chosen for it belongs to that
// attribute. Only active attributes can be glued to a product.
func verifyAttributes(ctx context.Context, tx *ent.Tx, appID int, assignments []AttributeAssignmentRequest) error {
	if len(assignments) == 0 {
		return nil
	}
	ids := make([]int, len(assignments))
	for i, a := range assignments {
		ids[i] = a.AttributeID
	}
	attrs, err := tx.Attribute.
		Query().
		Where(
			entattribute.IDIn(ids...),
			entattribute.AppID(appID),
			entattribute.Status(1),
		).
		WithOptions().
		All(ctx)
	if err != nil {
		return fmt.Errorf("verify attributes: %w", err)
	}
	if len(attrs) != len(ids) {
		return ErrAttributeInvalid
	}

	// Map each attribute to the set of option ids it owns, so every chosen
	// option can be validated against its attribute's live catalogue.
	validOptions := make(map[int]map[int]struct{}, len(attrs))
	for _, a := range attrs {
		set := make(map[int]struct{}, len(a.Edges.Options))
		for _, o := range a.Edges.Options {
			set[o.ID] = struct{}{}
		}
		validOptions[a.ID] = set
	}
	for _, asg := range assignments {
		set := validOptions[asg.AttributeID]
		for _, o := range asg.Options {
			if _, ok := set[o.OptionID]; !ok {
				return ErrOptionInvalid
			}
		}
	}
	return nil
}

func createAssignments(ctx context.Context, tx *ent.Tx, productID int, assignments []AttributeAssignmentRequest) error {
	if len(assignments) == 0 {
		return nil
	}
	bulk := make([]*ent.ProductAttributeCreate, len(assignments))
	for i, a := range assignments {
		opts := make([]schema.ProductAttributeOption, len(a.Options))
		for j, o := range a.Options {
			opts[j] = schema.ProductAttributeOption{
				OptionID:   o.OptionID,
				PriceDelta: o.PriceDelta,
			}
		}
		bulk[i] = tx.ProductAttribute.
			Create().
			SetProductID(productID).
			SetAttributeID(a.AttributeID).
			SetIsMandatory(a.IsMandatory).
			SetOptions(opts)
	}
	if _, err := tx.ProductAttribute.CreateBulk(bulk...).Save(ctx); err != nil {
		return fmt.Errorf("create product attributes: %w", err)
	}
	return nil
}

func (r *productRepository) Create(ctx context.Context, item Product, assignments []AttributeAssignmentRequest) (Product, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("begin tx: %w", err)
	}
	if err := verifyAttributes(ctx, tx, item.AppID, assignments); err != nil {
		return Product{}, rollback(tx, err)
	}
	e, err := tx.Product.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetName(item.Name).
		SetPrice(item.Price).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Product{}, rollback(tx, fmt.Errorf("create product: %w", err))
	}
	if err := createAssignments(ctx, tx, e.ID, assignments); err != nil {
		return Product{}, rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, item.AppID, item.UserID, e.ID)
}

func (r *productRepository) List(ctx context.Context, appID, userID, limit, offset int, status *int8) ([]Product, error) {
	q := r.client.Product.
		Query().
		Where(entproduct.AppID(appID))
	if status != nil {
		q = q.Where(entproduct.Status(*status))
	}
	es, err := q.
		Order(ent.Asc(entproduct.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	items := make([]Product, len(es))
	for i, e := range es {
		items[i] = r.mapToModel(e)
	}
	return items, nil
}

func (r *productRepository) GetByID(ctx context.Context, appID, userID, id int) (Product, error) {
	e, err := r.client.Product.
		Query().
		Where(entproduct.ID(id), entproduct.AppID(appID)).
		WithAttributes(func(paq *ent.ProductAttributeQuery) {
			paq.WithAttribute(func(aq *ent.AttributeQuery) {
				aq.WithOptions(func(oq *ent.AttributeOptionQuery) {
					oq.Order(ent.Asc(attributeoption.FieldID))
				})
			})
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("get product by id: %w", err)
	}
	item := r.mapToModel(e)
	item.Attributes = mapAssignments(e.Edges.Attributes)
	return item, nil
}

func (r *productRepository) Update(ctx context.Context, appID, userID, id int, item Product, assignments []AttributeAssignmentRequest) (Product, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("begin tx: %w", err)
	}
	if err := verifyAttributes(ctx, tx, appID, assignments); err != nil {
		return Product{}, rollback(tx, err)
	}
	count, err := tx.Product.
		Update().
		Where(entproduct.ID(id), entproduct.AppID(appID)).
		SetName(item.Name).
		SetPrice(item.Price).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Product{}, rollback(tx, fmt.Errorf("update product: %w", err))
	}
	if count == 0 {
		return Product{}, rollback(tx, ErrProductNotFound)
	}
	// Full sync: replace existing assignments with the requested set.
	if _, err := tx.ProductAttribute.
		Delete().
		Where(productattribute.ProductID(id)).
		Exec(ctx); err != nil {
		return Product{}, rollback(tx, fmt.Errorf("delete product attributes: %w", err))
	}
	if err := createAssignments(ctx, tx, id, assignments); err != nil {
		return Product{}, rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, appID, userID, id)
}

func (r *productRepository) Delete(ctx context.Context, appID, userID, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.Product.
		Query().
		Where(entproduct.ID(id), entproduct.AppID(appID)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check product exists: %w", err))
	}
	if !exists {
		return rollback(tx, ErrProductNotFound)
	}
	// No FK ties orders to products by design, so guard explicitly: a product
	// referenced by any order item cannot be deleted.
	used, err := tx.OrderProduct.
		Query().
		Where(orderproduct.ProductID(id)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check product in use: %w", err))
	}
	if used {
		return rollback(tx, ErrProductInUse)
	}
	if _, err := tx.ProductAttribute.
		Delete().
		Where(productattribute.ProductID(id)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete product attributes: %w", err))
	}
	if _, err := tx.Product.
		Delete().
		Where(entproduct.ID(id), entproduct.AppID(appID)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete product: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *productRepository) mapToModel(e *ent.Product) Product {
	return Product{
		ID:        e.ID,
		AppID:     e.AppID,
		UserID:    e.UserID,
		Name:      e.Name,
		Price:     e.Price,
		Status:    e.Status,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

func mapAssignments(pas []*ent.ProductAttribute) []AssignedAttribute {
	assigned := make([]AssignedAttribute, 0, len(pas))
	for _, pa := range pas {
		a := AssignedAttribute{
			AttributeID: pa.AttributeID,
			IsMandatory: pa.IsMandatory,
			Options:     []AttributeOption{},
		}
		if attr := pa.Edges.Attribute; attr != nil {
			a.Name = attr.Name
			// Resolve the option value from the live catalogue; the allowed
			// subset and its price deltas come from the product-attribute row.
			valueByID := make(map[int]string, len(attr.Edges.Options))
			for _, o := range attr.Edges.Options {
				valueByID[o.ID] = o.Value
			}
			for _, po := range pa.Options {
				a.Options = append(a.Options, AttributeOption{
					ID:         po.OptionID,
					Value:      valueByID[po.OptionID],
					PriceDelta: po.PriceDelta,
				})
			}
		}
		assigned = append(assigned, a)
	}
	return assigned
}

func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return fmt.Errorf("%w: rollback: %v", err, rerr)
	}
	return err
}
