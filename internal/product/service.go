package product

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

var ErrProductNotFound = errors.New("product not found")

type Repository interface {
	Create(ctx context.Context, item Product) (Product, error)
	List(ctx context.Context, appID, userID, limit, offset int) ([]Product, error)
	GetByID(ctx context.Context, appID, userID, id int) (Product, error)
	Update(ctx context.Context, appID, userID, id int, item Product) (Product, error)
	Delete(ctx context.Context, appID, userID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID int, req CreateProductRequest) (Product, error)
	List(ctx context.Context, appID, userID, limit, offset int) ([]Product, error)
	GetByID(ctx context.Context, appID, userID, id int) (Product, error)
	Update(ctx context.Context, appID, userID, id int, req UpdateProductRequest) (Product, error)
	Delete(ctx context.Context, appID, userID, id int) error
}

type service struct {
	repo     Repository
	validate *validator.Validate
}

func NewService(repo Repository) Service {
	return &service{
		repo:     repo,
		validate: validator.New(),
	}
}

func (s *service) Create(ctx context.Context, appID, userID int, req CreateProductRequest) (Product, error) {
	if err := s.validate.Struct(req); err != nil {
		return Product{}, fmt.Errorf("validate request: %w", err)
	}
	if req.Status == 0 {
		req.Status = 1
	}
	item := Product{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Status: req.Status,
	}
	created, err := s.repo.Create(ctx, item)
	if err != nil {
		slog.Error("failed to create product", "error", err, "app_id", appID, "user_id", userID)
		return Product{}, err
	}
	slog.Info("product created", "id", created.ID, "app_id", appID, "user_id", userID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, userID, limit, offset int) ([]Product, error) {
	items, err := s.repo.List(ctx, appID, userID, limit, offset)
	if err != nil {
		slog.Error("failed to list products", "error", err, "app_id", appID, "user_id", userID)
		return nil, err
	}
	return items, nil
}

func (s *service) GetByID(ctx context.Context, appID, userID, id int) (Product, error) {
	item, err := s.repo.GetByID(ctx, appID, userID, id)
	if err != nil {
		if !errors.Is(err, ErrProductNotFound) {
			slog.Error("failed to get product by id", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Product{}, err
	}
	return item, nil
}

func (s *service) Update(ctx context.Context, appID, userID, id int, req UpdateProductRequest) (Product, error) {
	if err := s.validate.Struct(req); err != nil {
		return Product{}, fmt.Errorf("validate request: %w", err)
	}
	item := Product{
		Name:   req.Name,
		Status: req.Status,
	}
	updated, err := s.repo.Update(ctx, appID, userID, id, item)
	if err != nil {
		if !errors.Is(err, ErrProductNotFound) {
			slog.Error("failed to update product", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Product{}, err
	}
	slog.Info("product updated", "id", updated.ID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, appID, userID, id int) error {
	if err := s.repo.Delete(ctx, appID, userID, id); err != nil {
		if !errors.Is(err, ErrProductNotFound) {
			slog.Error("failed to delete product", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return err
	}
	slog.Info("product deleted", "id", id, "app_id", appID, "user_id", userID)
	return nil
}
