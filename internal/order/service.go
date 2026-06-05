package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/go-playground/validator/v10"
)

var (
	ErrOrderNotFound             = errors.New("order not found")
	ErrOrderItemNotFound         = errors.New("order item not found")
	ErrOrderItemImmutable        = errors.New("existing order item product and attributes cannot be changed")
	ErrInvalidOrderItem          = errors.New("order item must have either id (existing) or product_id (new)")
	ErrDuplicateOrderItem        = errors.New("duplicate order item id in request")
	ErrProductInvalid            = errors.New("product not found")
	ErrProductInactive           = errors.New("product is not active")
	ErrAttributeNotAssigned      = errors.New("attribute is not assigned to the product")
	ErrInvalidOption             = errors.New("option does not belong to the attribute")
	ErrMandatoryAttributeMissing = errors.New("mandatory attribute is missing")
	ErrDuplicateItemAttribute    = errors.New("duplicate attribute in order item")
	ErrInvalidCustomerContact    = errors.New("invalid customer contact number")
)

// customer contact: digits with optional leading +, spaces and dashes allowed.
var contactPattern = regexp.MustCompile(`^\+?[0-9][0-9 -]{5,18}$`)

type Repository interface {
	Create(ctx context.Context, item Order, items []OrderItemRequest) (Order, error)
	List(ctx context.Context, appID, userID, limit, offset int, status *int8) ([]Order, error)
	GetByID(ctx context.Context, appID, userID, id int) (Order, error)
	Update(ctx context.Context, appID, userID, id int, item Order, items []SyncOrderItemRequest) (Order, error)
	UpdateStatus(ctx context.Context, appID, userID, id int, status int8) (Order, error)
	Delete(ctx context.Context, appID, userID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID, divisionID int, req CreateOrderRequest) (Order, error)
	List(ctx context.Context, appID, userID, limit, offset int, status *int8) ([]Order, error)
	GetByID(ctx context.Context, appID, userID, id int) (Order, error)
	Update(ctx context.Context, appID, userID, id int, req UpdateOrderRequest) (Order, error)
	UpdateStatus(ctx context.Context, appID, userID, id int, req UpdateOrderStatusRequest) (Order, error)
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

// isItemError reports whether err is an expected business validation error
// (not worth an error-level log).
func isItemError(err error) bool {
	return errors.Is(err, ErrProductInvalid) ||
		errors.Is(err, ErrProductInactive) ||
		errors.Is(err, ErrAttributeNotAssigned) ||
		errors.Is(err, ErrInvalidOption) ||
		errors.Is(err, ErrMandatoryAttributeMissing) ||
		errors.Is(err, ErrDuplicateItemAttribute) ||
		errors.Is(err, ErrOrderItemNotFound) ||
		errors.Is(err, ErrOrderItemImmutable) ||
		errors.Is(err, ErrInvalidOrderItem) ||
		errors.Is(err, ErrDuplicateOrderItem)
}

func (s *service) Create(ctx context.Context, appID, userID, divisionID int, req CreateOrderRequest) (Order, error) {
	if err := s.validate.Struct(req); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	if !contactPattern.MatchString(req.CustomerContact) {
		return Order{}, ErrInvalidCustomerContact
	}
	status := StatusPending
	if req.Status != nil {
		status = *req.Status
	}
	item := Order{
		AppID:           appID,
		UserID:          userID,
		DivisionID:      divisionID,
		CustomerName:    req.CustomerName,
		CustomerContact: req.CustomerContact,
		Status:          status,
	}
	created, err := s.repo.Create(ctx, item, req.Products)
	if err != nil {
		if !isItemError(err) {
			slog.Error("failed to create order", "error", err, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order created", "id", created.ID, "app_id", appID, "user_id", userID, "division_id", divisionID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, userID, limit, offset int, status *int8) ([]Order, error) {
	items, err := s.repo.List(ctx, appID, userID, limit, offset, status)
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
	if !contactPattern.MatchString(req.CustomerContact) {
		return Order{}, ErrInvalidCustomerContact
	}
	seen := make(map[int]struct{}, len(req.Products))
	for _, p := range req.Products {
		switch {
		case p.ID > 0 && p.ProductID > 0,
			p.ID == 0 && p.ProductID == 0:
			return Order{}, ErrInvalidOrderItem
		case p.ID > 0 && len(p.Attributes) > 0:
			// Existing items keep their snapshot; only quantity may change.
			return Order{}, ErrOrderItemImmutable
		case p.ID > 0:
			if _, ok := seen[p.ID]; ok {
				return Order{}, ErrDuplicateOrderItem
			}
			seen[p.ID] = struct{}{}
		}
	}
	item := Order{
		CustomerName:    req.CustomerName,
		CustomerContact: req.CustomerContact,
	}
	updated, err := s.repo.Update(ctx, appID, userID, id, item, req.Products)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) && !isItemError(err) {
			slog.Error("failed to update order", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order updated", "id", updated.ID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) UpdateStatus(ctx context.Context, appID, userID, id int, req UpdateOrderStatusRequest) (Order, error) {
	if err := s.validate.Struct(req); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	updated, err := s.repo.UpdateStatus(ctx, appID, userID, id, req.Status)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to update order status", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order status updated", "id", updated.ID, "status", req.Status, "app_id", appID, "user_id", userID)
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
