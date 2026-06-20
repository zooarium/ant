package attribute

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	ErrAttributeNotFound    = errors.New("attribute not found")
	ErrAttributeInUse       = errors.New("attribute is assigned to one or more products")
	ErrOptionNotFound       = errors.New("attribute option not found")
	ErrDuplicateOptionValue = errors.New("duplicate option value for attribute")
)

type Repository interface {
	Create(ctx context.Context, item Attribute) (Attribute, error)
	List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Attribute, error)
	GetByID(ctx context.Context, appID, userID, divisionID, id int) (Attribute, error)
	Update(ctx context.Context, appID, userID, divisionID, id int, item Attribute, options []SyncOptionRequest) (Attribute, error)
	Delete(ctx context.Context, appID, userID, divisionID, id int) error
}

type Service interface {
	Create(ctx context.Context, appID, userID, divisionID int, req CreateAttributeRequest) (Attribute, error)
	List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Attribute, error)
	GetByID(ctx context.Context, appID, userID, divisionID, id int) (Attribute, error)
	Update(ctx context.Context, appID, userID, divisionID, id int, req UpdateAttributeRequest) (Attribute, error)
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

func checkDuplicateValues(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		key := strings.ToLower(strings.TrimSpace(v))
		if _, ok := seen[key]; ok {
			return ErrDuplicateOptionValue
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (s *service) Create(ctx context.Context, appID, userID, divisionID int, req CreateAttributeRequest) (Attribute, error) {
	if err := s.validate.Struct(req); err != nil {
		return Attribute{}, fmt.Errorf("validate request: %w", err)
	}
	values := make([]string, len(req.Options))
	options := make([]Option, len(req.Options))
	for i, o := range req.Options {
		values[i] = o.Value
		options[i] = Option{Value: o.Value}
	}
	if err := checkDuplicateValues(values); err != nil {
		return Attribute{}, err
	}
	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}
	item := Attribute{
		AppID:      appID,
		UserID:     userID,
		DivisionID: divisionID,
		Name:       req.Name,
		Status:     status,
		Options:    options,
	}
	created, err := s.repo.Create(ctx, item)
	if err != nil {
		slog.Error("failed to create attribute", "error", err, "app_id", appID, "user_id", userID)
		return Attribute{}, err
	}
	slog.Info("attribute created", "id", created.ID, "app_id", appID, "user_id", userID)
	return created, nil
}

func (s *service) List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Attribute, error) {
	items, err := s.repo.List(ctx, appID, userID, divisionID, limit, offset, status)
	if err != nil {
		slog.Error("failed to list attributes", "error", err, "app_id", appID, "user_id", userID)
		return nil, err
	}
	return items, nil
}

func (s *service) GetByID(ctx context.Context, appID, userID, divisionID, id int) (Attribute, error) {
	item, err := s.repo.GetByID(ctx, appID, userID, divisionID, id)
	if err != nil {
		if !errors.Is(err, ErrAttributeNotFound) {
			slog.Error("failed to get attribute by id", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Attribute{}, err
	}
	return item, nil
}

func (s *service) Update(ctx context.Context, appID, userID, divisionID, id int, req UpdateAttributeRequest) (Attribute, error) {
	if err := s.validate.Struct(req); err != nil {
		return Attribute{}, fmt.Errorf("validate request: %w", err)
	}
	values := make([]string, len(req.Options))
	for i, o := range req.Options {
		values[i] = o.Value
	}
	if err := checkDuplicateValues(values); err != nil {
		return Attribute{}, err
	}
	item := Attribute{
		Name:   req.Name,
		Status: *req.Status,
	}
	updated, err := s.repo.Update(ctx, appID, userID, divisionID, id, item, req.Options)
	if err != nil {
		if !errors.Is(err, ErrAttributeNotFound) && !errors.Is(err, ErrOptionNotFound) {
			slog.Error("failed to update attribute", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return Attribute{}, err
	}
	slog.Info("attribute updated", "id", updated.ID, "app_id", appID, "user_id", userID)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, appID, userID, divisionID, id int) error {
	if err := s.repo.Delete(ctx, appID, userID, divisionID, id); err != nil {
		if !errors.Is(err, ErrAttributeNotFound) && !errors.Is(err, ErrAttributeInUse) {
			slog.Error("failed to delete attribute", "error", err, "id", id, "app_id", appID, "user_id", userID)
		}
		return err
	}
	slog.Info("attribute deleted", "id", id, "app_id", appID, "user_id", userID)
	return nil
}
