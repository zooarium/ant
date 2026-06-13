package ordergroup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var (
	ErrOrderGroupNotFound = errors.New("order group not found")
	ErrOrderGroupInUse    = errors.New("order group has orders and cannot be deleted")
)

type Repository interface {
	Create(ctx context.Context, item OrderGroup) (OrderGroup, error)
	List(ctx context.Context, appID, limit, offset int, status *int8) ([]OrderGroup, error)
	GetByID(ctx context.Context, appID, id int) (OrderGroup, error)
	GetByToken(ctx context.Context, appID int, token string) (OrderGroup, error)
	ListByDevice(ctx context.Context, appID, divisionID int, deviceID string, limit, offset int) ([]OrderGroup, error)
	Update(ctx context.Context, appID, id int, label string) (OrderGroup, error)
	UpdateStatus(ctx context.Context, appID, id int, status int8) (OrderGroup, error)
	Delete(ctx context.Context, appID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID, divisionID int, req CreateOrderGroupRequest) (OrderGroup, error)
	List(ctx context.Context, appID, limit, offset int, status *int8) ([]OrderGroup, error)
	GetByID(ctx context.Context, appID, id int) (OrderGroup, error)
	GetByToken(ctx context.Context, appID int, token string) (OrderGroup, error)
	ListByDevice(ctx context.Context, appID, divisionID int, deviceID string, limit, offset int) ([]OrderGroup, error)
	Update(ctx context.Context, appID, id int, req UpdateOrderGroupRequest) (OrderGroup, error)
	UpdateStatus(ctx context.Context, appID, id int, req UpdateOrderGroupStatusRequest) (OrderGroup, error)
	Delete(ctx context.Context, appID, id int) error
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

func (s *service) Create(ctx context.Context, appID, userID, divisionID int, req CreateOrderGroupRequest) (OrderGroup, error) {
	if err := s.validate.Struct(req); err != nil {
		return OrderGroup{}, fmt.Errorf("validate request: %w", err)
	}
	item := OrderGroup{
		AppID:      appID,
		UserID:     userID,
		DivisionID: divisionID,
		Token:      uuid.NewString(),
		Label:      req.Label,
		Status:     StatusOpen,
	}
	created, err := s.repo.Create(ctx, item)
	if err != nil {
		slog.Error("failed to create order group", "error", err, "app_id", appID, "user_id", userID)
		return OrderGroup{}, err
	}
	slog.Info("order group created", "id", created.ID, "app_id", appID, "user_id", userID, "division_id", divisionID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, limit, offset int, status *int8) ([]OrderGroup, error) {
	items, err := s.repo.List(ctx, appID, limit, offset, status)
	if err != nil {
		slog.Error("failed to list order groups", "error", err, "app_id", appID)
		return nil, err
	}
	return items, nil
}

func (s *service) GetByID(ctx context.Context, appID, id int) (OrderGroup, error) {
	item, err := s.repo.GetByID(ctx, appID, id)
	if err != nil {
		if !errors.Is(err, ErrOrderGroupNotFound) {
			slog.Error("failed to get order group by id", "error", err, "id", id, "app_id", appID)
		}
		return OrderGroup{}, err
	}
	return item, nil
}

// GetByToken returns a tab by its shareable token, scoped to the tenant. Used
// by the public order-intake page.
func (s *service) GetByToken(ctx context.Context, appID int, token string) (OrderGroup, error) {
	item, err := s.repo.GetByToken(ctx, appID, token)
	if err != nil {
		if !errors.Is(err, ErrOrderGroupNotFound) {
			slog.Error("failed to get order group by token", "error", err, "app_id", appID)
		}
		return OrderGroup{}, err
	}
	return item, nil
}

// ListByDevice returns past tabs for a device (newest first), each with its
// orders and combined total. Backs the public order history view.
func (s *service) ListByDevice(ctx context.Context, appID, divisionID int, deviceID string, limit, offset int) ([]OrderGroup, error) {
	items, err := s.repo.ListByDevice(ctx, appID, divisionID, deviceID, limit, offset)
	if err != nil {
		slog.Error("failed to list order groups by device", "error", err, "app_id", appID, "division_id", divisionID)
		return nil, err
	}
	return items, nil
}

func (s *service) Update(ctx context.Context, appID, id int, req UpdateOrderGroupRequest) (OrderGroup, error) {
	if err := s.validate.Struct(req); err != nil {
		return OrderGroup{}, fmt.Errorf("validate request: %w", err)
	}
	updated, err := s.repo.Update(ctx, appID, id, req.Label)
	if err != nil {
		if !errors.Is(err, ErrOrderGroupNotFound) {
			slog.Error("failed to update order group", "error", err, "id", id, "app_id", appID)
		}
		return OrderGroup{}, err
	}
	slog.Info("order group updated", "id", updated.ID, "app_id", appID)
	return updated, nil
}

func (s *service) UpdateStatus(ctx context.Context, appID, id int, req UpdateOrderGroupStatusRequest) (OrderGroup, error) {
	if err := s.validate.Struct(req); err != nil {
		return OrderGroup{}, fmt.Errorf("validate request: %w", err)
	}
	updated, err := s.repo.UpdateStatus(ctx, appID, id, req.Status)
	if err != nil {
		if !errors.Is(err, ErrOrderGroupNotFound) {
			slog.Error("failed to update order group status", "error", err, "id", id, "app_id", appID)
		}
		return OrderGroup{}, err
	}
	slog.Info("order group status updated", "id", updated.ID, "status", req.Status, "app_id", appID)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, appID, id int) error {
	if err := s.repo.Delete(ctx, appID, id); err != nil {
		if !errors.Is(err, ErrOrderGroupNotFound) && !errors.Is(err, ErrOrderGroupInUse) {
			slog.Error("failed to delete order group", "error", err, "id", id, "app_id", appID)
		}
		return err
	}
	slog.Info("order group deleted", "id", id, "app_id", appID)
	return nil
}
