package product

import (
	"context"
	"fmt"

	"ant/ent"
	entattribute "ant/ent/attribute"
	"ant/ent/attributeoption"
	entcategory "ant/ent/category"
	"ant/ent/orderproduct"
	entproduct "ant/ent/product"
	"ant/ent/productattribute"
	"ant/ent/schema"
	"ant/internal/category"
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
func verifyAttributes(ctx context.Context, tx *ent.Tx, appID, divisionID int, assignments []AttributeAssignmentRequest) error {
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
			entattribute.DivisionID(divisionID),
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

// verifyCategory ensures the assigned category (if any) exists, belongs to the
// app and is active. A nil categoryID means "no category" and always passes.
func verifyCategory(ctx context.Context, tx *ent.Tx, appID, divisionID int, categoryID *int) error {
	if categoryID == nil {
		return nil
	}
	ok, err := tx.Category.
		Query().
		Where(
			entcategory.IDEQ(*categoryID),
			entcategory.AppIDEQ(appID),
			entcategory.DivisionIDEQ(divisionID),
			entcategory.StatusEQ(1),
		).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("verify category: %w", err)
	}
	if !ok {
		return ErrCategoryInvalid
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
	if err := verifyAttributes(ctx, tx, item.AppID, item.DivisionID, assignments); err != nil {
		return Product{}, rollback(tx, err)
	}
	if err := verifyCategory(ctx, tx, item.AppID, item.DivisionID, item.CategoryID); err != nil {
		return Product{}, rollback(tx, err)
	}
	e, err := tx.Product.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetDivisionID(item.DivisionID).
		SetName(item.Name).
		SetPrice(item.Price).
		SetStatus(item.Status).
		SetNillableCategoryID(item.CategoryID).
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
	return r.GetByID(ctx, item.AppID, item.UserID, item.DivisionID, e.ID)
}

func (r *productRepository) List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8, categoryID *int) ([]Product, error) {
	q := r.client.Product.
		Query().
		Where(entproduct.AppID(appID), entproduct.DivisionID(divisionID))
	if status != nil {
		q = q.Where(entproduct.Status(*status))
	}
	if categoryID != nil {
		// Hierarchical filter: include the category and its whole subtree. An
		// unknown category yields an empty result rather than an error.
		ids, err := r.categorySubtreeIDs(ctx, appID, divisionID, *categoryID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []Product{}, nil
		}
		q = q.Where(entproduct.CategoryIDIn(ids...))
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
	if err := r.decorateCategories(ctx, appID, divisionID, items); err != nil {
		return nil, err
	}
	return items, nil
}

// categorySubtreeIDs returns the ids of the category and all its descendants,
// scoped to the app and division. An empty slice means the category does not exist.
func (r *productRepository) categorySubtreeIDs(ctx context.Context, appID, divisionID, categoryID int) ([]int, error) {
	cat, err := r.client.Category.
		Query().
		Where(entcategory.IDEQ(categoryID), entcategory.AppIDEQ(appID), entcategory.DivisionIDEQ(divisionID)).
		Select(entcategory.FieldPath).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve category subtree: %w", err)
	}
	ids, err := r.client.Category.
		Query().
		Where(entcategory.AppIDEQ(appID), entcategory.DivisionIDEQ(divisionID), entcategory.PathHasPrefix(cat.Path)).
		Select(entcategory.FieldID).
		Ints(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve category subtree ids: %w", err)
	}
	return ids, nil
}

func (r *productRepository) GetByID(ctx context.Context, appID, userID, divisionID, id int) (Product, error) {
	e, err := r.client.Product.
		Query().
		Where(entproduct.ID(id), entproduct.AppID(appID), entproduct.DivisionID(divisionID)).
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
	single := []Product{item}
	if err := r.decorateCategories(ctx, appID, divisionID, single); err != nil {
		return Product{}, err
	}
	return single[0], nil
}

func (r *productRepository) Update(ctx context.Context, appID, userID, divisionID, id int, item Product, assignments []AttributeAssignmentRequest) (Product, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("begin tx: %w", err)
	}
	if err := verifyAttributes(ctx, tx, appID, divisionID, assignments); err != nil {
		return Product{}, rollback(tx, err)
	}
	if err := verifyCategory(ctx, tx, appID, divisionID, item.CategoryID); err != nil {
		return Product{}, rollback(tx, err)
	}
	upd := tx.Product.
		Update().
		Where(entproduct.ID(id), entproduct.AppID(appID), entproduct.DivisionID(divisionID)).
		SetName(item.Name).
		SetPrice(item.Price).
		SetStatus(item.Status)
	// Full sync: a nil category_id clears any existing assignment.
	if item.CategoryID != nil {
		upd = upd.SetCategoryID(*item.CategoryID)
	} else {
		upd = upd.ClearCategoryID()
	}
	count, err := upd.Save(ctx)
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
	return r.GetByID(ctx, appID, userID, divisionID, id)
}

func (r *productRepository) Delete(ctx context.Context, appID, userID, divisionID, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.Product.
		Query().
		Where(entproduct.ID(id), entproduct.AppID(appID), entproduct.DivisionID(divisionID)).
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
		Where(entproduct.ID(id), entproduct.AppID(appID), entproduct.DivisionID(divisionID)).
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
		ID:         e.ID,
		AppID:      e.AppID,
		UserID:     e.UserID,
		DivisionID: e.DivisionID,
		Name:       e.Name,
		Price:      e.Price,
		Status:     e.Status,
		CategoryID: e.CategoryID,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

// decorateCategories fills the Category ref (id, name, display) for every
// product that has a category, resolving names and ancestor hierarchies in two
// batched queries (categories, then their ancestors) — no N+1.
func (r *productRepository) decorateCategories(ctx context.Context, appID, divisionID int, items []Product) error {
	catIDs := make(map[int]struct{})
	for _, it := range items {
		if it.CategoryID != nil {
			catIDs[*it.CategoryID] = struct{}{}
		}
	}
	if len(catIDs) == 0 {
		return nil
	}

	cats, err := r.client.Category.
		Query().
		Where(entcategory.AppIDEQ(appID), entcategory.DivisionIDEQ(divisionID), entcategory.IDIn(intKeys(catIDs)...)).
		Select(entcategory.FieldID, entcategory.FieldName, entcategory.FieldPath).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load product categories: %w", err)
	}

	type catInfo struct {
		name string
		path string
	}
	infoByID := make(map[int]catInfo, len(cats))
	ancestorIDs := make(map[int]struct{})
	for _, c := range cats {
		infoByID[c.ID] = catInfo{name: c.Name, path: c.Path}
		ids := category.ParsePathIDs(c.Path)
		if len(ids) > 1 {
			for _, a := range ids[:len(ids)-1] {
				ancestorIDs[a] = struct{}{}
			}
		}
	}

	nameByID := make(map[int]string)
	if len(ancestorIDs) > 0 {
		rows, err := r.client.Category.
			Query().
			Where(entcategory.AppIDEQ(appID), entcategory.DivisionIDEQ(divisionID), entcategory.IDIn(intKeys(ancestorIDs)...)).
			Select(entcategory.FieldID, entcategory.FieldName).
			All(ctx)
		if err != nil {
			return fmt.Errorf("load category ancestors: %w", err)
		}
		for _, row := range rows {
			nameByID[row.ID] = row.Name
		}
	}

	refByID := make(map[int]CategoryRef, len(cats))
	for id, info := range infoByID {
		ids := category.ParsePathIDs(info.path)
		var ancestors []string
		if len(ids) > 1 {
			for _, aid := range ids[:len(ids)-1] {
				if n, ok := nameByID[aid]; ok {
					ancestors = append(ancestors, n)
				}
			}
		}
		refByID[id] = CategoryRef{
			ID:      id,
			Name:    info.name,
			Display: category.BuildDisplay(info.name, ancestors),
		}
	}

	for i := range items {
		if items[i].CategoryID == nil {
			continue
		}
		if ref, ok := refByID[*items[i].CategoryID]; ok {
			ref := ref
			items[i].Category = &ref
		}
	}
	return nil
}

func intKeys(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
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
