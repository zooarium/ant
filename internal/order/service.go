package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

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
	ErrOrderGroupInvalid         = errors.New("order group not found")
	ErrPublicOrderLimit          = errors.New("public order limit reached for this device")
)

// customer contact: digits with optional leading +, spaces and dashes allowed.
var contactPattern = regexp.MustCompile(`^\+?[0-9][0-9 -]{5,18}$`)

type Repository interface {
	Create(ctx context.Context, item Order, items []OrderItemRequest, groupID *int, groupLabel string) (Order, error)
	CreatePublic(ctx context.Context, item Order, items []OrderItemRequest, groupID *int, groupLabel string, maxOrders int, window time.Duration) (Order, error)
	List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Order, error)
	GetByID(ctx context.Context, appID, userID, divisionID, id int) (Order, error)
	Update(ctx context.Context, appID, userID, divisionID, id int, item Order, items []SyncOrderItemRequest, taxPercent *float64) (Order, error)
	UpdateStatus(ctx context.Context, appID, userID, divisionID, id int, status int8) (Order, error)
	SetGroup(ctx context.Context, appID, userID, divisionID, id, groupID int) (Order, error)
	Delete(ctx context.Context, appID, userID, divisionID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID, divisionID int, ipAddress string, req CreateOrderRequest) (Order, error)
	CreatePublic(ctx context.Context, appID, userID, divisionID int, ipAddress string, req CreatePublicOrderRequest, maxOrders int, window time.Duration) (Order, error)
	List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Order, error)
	GetByID(ctx context.Context, appID, userID, divisionID, id int) (Order, error)
	Update(ctx context.Context, appID, userID, divisionID, id int, req UpdateOrderRequest) (Order, error)
	UpdateStatus(ctx context.Context, appID, userID, divisionID, id int, req UpdateOrderStatusRequest) (Order, error)
	SetGroup(ctx context.Context, appID, userID, divisionID, id int, req SetOrderGroupRequest) (Order, error)
	Delete(ctx context.Context, appID, userID, divisionID, id int) error
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
		errors.Is(err, ErrDuplicateOrderItem) ||
		errors.Is(err, ErrOrderGroupInvalid)
}

func (s *service) Create(ctx context.Context, appID, userID, divisionID int, ipAddress string, req CreateOrderRequest) (Order, error) {
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
		IPAddress:       ipAddress,
		DeviceID:        req.DeviceID,
	}
	if req.TaxPercent != nil {
		item.TaxPercent = *req.TaxPercent
	}
	if req.OrderedAt != nil {
		item.OrderedAt = *req.OrderedAt
	}
	created, err := s.repo.Create(ctx, item, req.Products, req.GroupID, req.GroupLabel)
	if err != nil {
		if !isItemError(err) {
			slog.Error("failed to create order", "error", err, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order created", "id", created.ID, "app_id", appID, "user_id", userID, "division_id", divisionID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Order, error) {
	items, err := s.repo.List(ctx, appID, userID, divisionID, limit, offset, status)
	if err != nil {
		slog.Error("failed to list orders", "error", err, "app_id", appID, "user_id", userID)
		return nil, err
	}
	return items, nil
}

// CreatePublic creates an order from the public order-intake page on behalf of
// a guest token. It forces StatusPending (a guest can never self-confirm or
// mark paid — that is reception's job) and enforces a per-device volume cap
// (maxOrders within window) inside the repository transaction.
func (s *service) CreatePublic(ctx context.Context, appID, userID, divisionID int, ipAddress string, req CreatePublicOrderRequest, maxOrders int, window time.Duration) (Order, error) {
	if err := s.validate.Struct(req.CreateOrderRequest); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	if !contactPattern.MatchString(req.CustomerContact) {
		return Order{}, ErrInvalidCustomerContact
	}
	item := Order{
		AppID:           appID,
		UserID:          userID,
		DivisionID:      divisionID,
		CustomerName:    req.CustomerName,
		CustomerContact: req.CustomerContact,
		Status:          StatusPending,
		IPAddress:       ipAddress,
		DeviceID:        req.DeviceID,
	}
	if req.TaxPercent != nil {
		item.TaxPercent = *req.TaxPercent
	}
	if req.OrderedAt != nil {
		item.OrderedAt = *req.OrderedAt
	}
	created, err := s.repo.CreatePublic(ctx, item, req.Products, req.GroupID, req.GroupLabel, maxOrders, window)
	if err != nil {
		if !isItemError(err) && !errors.Is(err, ErrPublicOrderLimit) {
			slog.Error("failed to create public order", "error", err, "app_id", appID, "division_id", divisionID)
		}
		return Order{}, err
	}
	slog.Info("public order created", "id", created.ID, "app_id", appID, "division_id", divisionID)
	return created, nil
}

func (s *service) GetByID(ctx context.Context, appID, userID, divisionID, id int) (Order, error) {
	item, err := s.repo.GetByID(ctx, appID, userID, divisionID, id)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to get order by id", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	return item, nil
}

func (s *service) Update(ctx context.Context, appID, userID, divisionID, id int, req UpdateOrderRequest) (Order, error) {
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
	if req.OrderedAt != nil {
		item.OrderedAt = *req.OrderedAt
	}
	updated, err := s.repo.Update(ctx, appID, userID, divisionID, id, item, req.Products, req.TaxPercent)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) && !isItemError(err) {
			slog.Error("failed to update order", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order updated", "id", updated.ID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) UpdateStatus(ctx context.Context, appID, userID, divisionID, id int, req UpdateOrderStatusRequest) (Order, error) {
	if err := s.validate.Struct(req); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	updated, err := s.repo.UpdateStatus(ctx, appID, userID, divisionID, id, req.Status)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to update order status", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order status updated", "id", updated.ID, "status", req.Status, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) SetGroup(ctx context.Context, appID, userID, divisionID, id int, req SetOrderGroupRequest) (Order, error) {
	if err := s.validate.Struct(req); err != nil {
		return Order{}, fmt.Errorf("validate request: %w", err)
	}
	updated, err := s.repo.SetGroup(ctx, appID, userID, divisionID, id, *req.GroupID)
	if err != nil {
		if !errors.Is(err, ErrOrderNotFound) && !errors.Is(err, ErrOrderGroupInvalid) {
			slog.Error("failed to set order group", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Order{}, err
	}
	slog.Info("order group set", "id", updated.ID, "group_id", req.GroupID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, appID, userID, divisionID, id int) error {
	if err := s.repo.Delete(ctx, appID, userID, divisionID, id); err != nil {
		if !errors.Is(err, ErrOrderNotFound) {
			slog.Error("failed to delete order", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return err
	}
	slog.Info("order deleted", "id", id, "app_id", appID, "user_id", userID)
	return nil
}
