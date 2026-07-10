package product

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

var (
	ErrProductNotFound    = errors.New("product not found")
	ErrProductInUse       = errors.New("product is used in one or more orders")
	ErrAttributeInvalid   = errors.New("attribute not found or not active")
	ErrDuplicateAttribute = errors.New("duplicate attribute in request")
	ErrOptionInvalid      = errors.New("option does not belong to the attribute")
	ErrDuplicateOption    = errors.New("duplicate option in attribute assignment")
	ErrCategoryInvalid    = errors.New("category not found or not active")
)

type Repository interface {
	Create(ctx context.Context, item Product, assignments []AttributeAssignmentRequest) (Product, error)
	List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8, categoryID *int, featured *bool) ([]Product, error)
	GetByID(ctx context.Context, appID, userID, divisionID, id int) (Product, error)
	Update(ctx context.Context, appID, userID, divisionID, id int, item Product, assignments []AttributeAssignmentRequest) (Product, error)
	Delete(ctx context.Context, appID, userID, divisionID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID, divisionID int, req CreateProductRequest) (Product, error)
	List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8, categoryID *int, featured *bool) ([]Product, error)
	GetByID(ctx context.Context, appID, userID, divisionID, id int) (Product, error)
	Update(ctx context.Context, appID, userID, divisionID, id int, req UpdateProductRequest) (Product, error)
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

func checkDuplicateAssignments(assignments []AttributeAssignmentRequest) error {
	seen := make(map[int]struct{}, len(assignments))
	for _, a := range assignments {
		if _, ok := seen[a.AttributeID]; ok {
			return ErrDuplicateAttribute
		}
		seen[a.AttributeID] = struct{}{}

		opts := make(map[int]struct{}, len(a.Options))
		for _, o := range a.Options {
			if _, ok := opts[o.OptionID]; ok {
				return ErrDuplicateOption
			}
			opts[o.OptionID] = struct{}{}
		}
	}
	return nil
}

func (s *service) Create(ctx context.Context, appID, userID, divisionID int, req CreateProductRequest) (Product, error) {
	if err := s.validate.Struct(req); err != nil {
		return Product{}, fmt.Errorf("validate request: %w", err)
	}
	if err := checkDuplicateAssignments(req.Attributes); err != nil {
		return Product{}, err
	}
	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}
	item := Product{
		AppID:      appID,
		UserID:     userID,
		DivisionID: divisionID,
		Name:       req.Name,
		Price:      req.Price,
		Status:     status,
		Featured:   req.Featured,
		CategoryID: req.CategoryID,
	}
	created, err := s.repo.Create(ctx, item, req.Attributes)
	if err != nil {
		if !errors.Is(err, ErrAttributeInvalid) && !errors.Is(err, ErrCategoryInvalid) {
			slog.Error("failed to create product", "error", err, "app_id", appID, "user_id", userID)
		}
		return Product{}, err
	}
	slog.Info("product created", "id", created.ID, "app_id", appID, "user_id", userID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8, categoryID *int, featured *bool) ([]Product, error) {
	items, err := s.repo.List(ctx, appID, userID, divisionID, limit, offset, status, categoryID, featured)
	if err != nil {
		slog.Error("failed to list products", "error", err, "app_id", appID, "user_id", userID)
		return nil, err
	}
	return items, nil
}

func (s *service) GetByID(ctx context.Context, appID, userID, divisionID, id int) (Product, error) {
	item, err := s.repo.GetByID(ctx, appID, userID, divisionID, id)
	if err != nil {
		if !errors.Is(err, ErrProductNotFound) {
			slog.Error("failed to get product by id", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Product{}, err
	}
	return item, nil
}

func (s *service) Update(ctx context.Context, appID, userID, divisionID, id int, req UpdateProductRequest) (Product, error) {
	if err := s.validate.Struct(req); err != nil {
		return Product{}, fmt.Errorf("validate request: %w", err)
	}
	if err := checkDuplicateAssignments(req.Attributes); err != nil {
		return Product{}, err
	}
	item := Product{
		Name:       req.Name,
		Price:      req.Price,
		Status:     *req.Status,
		Featured:   req.Featured,
		CategoryID: req.CategoryID,
	}
	updated, err := s.repo.Update(ctx, appID, userID, divisionID, id, item, req.Attributes)
	if err != nil {
		if !errors.Is(err, ErrProductNotFound) && !errors.Is(err, ErrAttributeInvalid) && !errors.Is(err, ErrCategoryInvalid) {
			slog.Error("failed to update product", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Product{}, err
	}
	slog.Info("product updated", "id", updated.ID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, appID, userID, divisionID, id int) error {
	if err := s.repo.Delete(ctx, appID, userID, divisionID, id); err != nil {
		if !errors.Is(err, ErrProductNotFound) && !errors.Is(err, ErrProductInUse) {
			slog.Error("failed to delete product", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return err
	}
	slog.Info("product deleted", "id", id, "app_id", appID, "user_id", userID)
	return nil
}
