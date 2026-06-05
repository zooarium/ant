package product

import (
	"context"
	"fmt"

	"ant/ent"
	"ant/ent/product"
)

type productRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *productRepository {
	return &productRepository{client: client}
}

func (r *productRepository) Create(ctx context.Context, item Product) (Product, error) {
	e, err := r.client.Product.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetName(item.Name).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	return r.mapToModel(e), nil
}

func (r *productRepository) List(ctx context.Context, appID, userID, limit, offset int) ([]Product, error) {
	es, err := r.client.Product.
		Query().
		Where(product.AppID(appID)).
		Order(ent.Asc(product.FieldID)).
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
		Where(product.ID(id), product.AppID(appID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("get product by id: %w", err)
	}
	return r.mapToModel(e), nil
}

func (r *productRepository) Update(ctx context.Context, appID, userID, id int, item Product) (Product, error) {
	count, err := r.client.Product.
		Update().
		Where(product.ID(id), product.AppID(appID)).
		SetName(item.Name).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("update product: %w", err)
	}
	if count == 0 {
		return Product{}, ErrProductNotFound
	}
	return r.GetByID(ctx, appID, userID, id)
}

func (r *productRepository) Delete(ctx context.Context, appID, userID, id int) error {
	count, err := r.client.Product.
		Delete().
		Where(product.ID(id), product.AppID(appID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	if count == 0 {
		return ErrProductNotFound
	}
	return nil
}

func (r *productRepository) mapToModel(e *ent.Product) Product {
	return Product{
		ID:        e.ID,
		AppID:     e.AppID,
		UserID:    e.UserID,
		Name:      e.Name,
		Status:    e.Status,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
