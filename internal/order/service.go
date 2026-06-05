package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

var ErrOrderNotFound = errors.New("order not found")

type Repository interface {
	Create(ctx context.Context, item Order) (Order, error)
	List(ctx context.Context, appID, userID, limit, offset int) ([]Order, error)
	GetByID(ctx context.Context, appID, userID, id int) (Order, error)
	Update(ctx context.Context, appID, userID, id int, item Order) (Order, error)
	Delete(ctx context.Context, appID, userID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID int, req CreateOrderRequest) (Order, error)
	List(ctx context.Context, appID, userID, limit, offset int) ([]Order, error)
	GetByID(ctx context.Context, appID, userID, id int) (Order, error)
	Update(ctx context.Context, appID, userID, id int, req UpdateOrderRequest) (Order, error)
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

func (s *service) Create(ctx context.Context, appID, userID int, req CreateOrderRequest) (Order, error) {
	if err := s.validate.Struct(req); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	if req.Status == 0 {
		req.Status = 1
	}
	item := Order{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Status: req.Status,
	}
	created, err := s.repo.Create(ctx, item)
	if err != nil {
		slog.Error("failed to create order", "error", err, "app_id", appID, "user_id", userID)
		return Order{}, err
	}
	slog.Info("order created", "id", created.ID, "app_id", appID, "user_id", userID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, userID, limit, offset int) ([]Order, error) {
	items, err := s.repo.List(ctx, appID, userID, limit, offset)
	if err != nil {
		slog.Error("failed to list orders", "error", err, "app_id", appID, "user_id", userID)
		return nil, err
	}
	return items, nil
}

func (s *service) GetByID(ctx context.Context, appID, userID, id int) (Order, error) {
	item, err := s.repo.GetByID(ctx, appID, userID, id)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to get order by id", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	return item, nil
}

func (s *service) Update(ctx context.Context, appID, userID, id int, req UpdateOrderRequest) (Order, error) {
	if err := s.validate.Struct(req); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	item := Order{
		Name:   req.Name,
		Status: req.Status,
	}
	updated, err := s.repo.Update(ctx, appID, userID, id, item)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to update order", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order updated", "id", updated.ID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, appID, userID, id int) error {
	if err := s.repo.Delete(ctx, appID, userID, id); err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to delete order", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return err
	}
	slog.Info("order deleted", "id", id, "app_id", appID, "user_id", userID)
	return nil
}
