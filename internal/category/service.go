package category

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

var (
	ErrCategoryNotFound = errors.New("category not found")
	ErrParentInactive   = errors.New("parent category is inactive")
	ErrParentNotFound   = errors.New("parent category not found")
	ErrMoveToSelf       = errors.New("cannot move category to itself")
	ErrMoveToDescendant = errors.New("cannot move category to its own descendant")
	ErrHasChildren      = errors.New("category has children; remove them first")
	ErrHasProducts      = errors.New("category has assigned products; reassign them first")
)

// Repository is the data-access contract for categories.
type Repository interface {
	Create(ctx context.Context, c Category, parentPath string) (*Category, error)
	GetByID(ctx context.Context, appID, id int) (*Category, error)
	List(ctx context.Context, appID int, parentID *int, status *int8, limit, offset int) ([]*Category, error)
	Descendants(ctx context.Context, appID int, path string) ([]*Category, error)
	Update(ctx context.Context, appID, id int, c *Category) (*Category, error)
	Move(ctx context.Context, id int, newParentID *int, oldPath, newPath string) error
	CountChildren(ctx context.Context, id int) (int, error)
	CountProducts(ctx context.Context, id int) (int, error)
	Delete(ctx context.Context, appID, id int) error
}

// Service is the business-logic contract for categories.
type Service interface {
	Create(ctx context.Context, appID int, req CreateCategoryRequest) (*Category, error)
	GetByID(ctx context.Context, appID, id int) (*Category, error)
	List(ctx context.Context, appID int, parentID *int, status *int8, limit, offset int) ([]*Category, error)
	Descendants(ctx context.Context, appID, id int) ([]*Category, error)
	Update(ctx context.Context, appID, id int, req UpdateCategoryRequest) (*Category, error)
	Move(ctx context.Context, appID, id int, req MoveCategoryRequest) (*Category, error)
	Delete(ctx context.Context, appID, id int) error
}

type service struct {
	repo Repository
}

// NewService creates a new category service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, appID int, req CreateCategoryRequest) (*Category, error) {
	parentPath := "/"
	if req.ParentID != nil {
		parent, err := s.repo.GetByID(ctx, appID, *req.ParentID)
		if err != nil {
			return nil, ErrParentNotFound
		}
		if parent.Status != 1 {
			return nil, ErrParentInactive
		}
		parentPath = parent.Path
	}

	created, err := s.repo.Create(ctx, Category{
		AppID:    appID,
		ParentID: req.ParentID,
		Name:     req.Name,
		Status:   1,
	}, parentPath)
	if err != nil {
		slog.Error("failed to create category", "name", req.Name, "app_id", appID, "error", err)
		return nil, err
	}
	slog.Info("category created", "id", created.ID, "app_id", appID, "path", created.Path)
	return created, nil
}

func (s *service) GetByID(ctx context.Context, appID, id int) (*Category, error) {
	return s.repo.GetByID(ctx, appID, id)
}

func (s *service) List(ctx context.Context, appID int, parentID *int, status *int8, limit, offset int) ([]*Category, error) {
	return s.repo.List(ctx, appID, parentID, status, limit, offset)
}

func (s *service) Descendants(ctx context.Context, appID, id int) ([]*Category, error) {
	c, err := s.repo.GetByID(ctx, appID, id)
	if err != nil {
		return nil, err
	}
	return s.repo.Descendants(ctx, appID, c.Path)
}

func (s *service) Update(ctx context.Context, appID, id int, req UpdateCategoryRequest) (*Category, error) {
	existing, err := s.repo.GetByID(ctx, appID, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}
	updated, err := s.repo.Update(ctx, appID, id, existing)
	if err != nil {
		slog.Error("failed to update category", "id", id, "app_id", appID, "error", err)
		return nil, err
	}
	slog.Info("category updated", "id", id, "app_id", appID)
	return updated, nil
}

func (s *service) Move(ctx context.Context, appID, id int, req MoveCategoryRequest) (*Category, error) {
	c, err := s.repo.GetByID(ctx, appID, id)
	if err != nil {
		return nil, err
	}

	newParentPath := "/"
	if req.ParentID != nil {
		if *req.ParentID == id {
			return nil, ErrMoveToSelf
		}
		newParent, err := s.repo.GetByID(ctx, appID, *req.ParentID)
		if err != nil {
			return nil, ErrParentNotFound
		}
		// New parent must not live inside the moved subtree.
		if len(newParent.Path) >= len(c.Path) && newParent.Path[:len(c.Path)] == c.Path {
			return nil, ErrMoveToDescendant
		}
		newParentPath = newParent.Path
	}

	oldPath := c.Path
	newPath := fmt.Sprintf("%s%d/", newParentPath, id)

	if err := s.repo.Move(ctx, id, req.ParentID, oldPath, newPath); err != nil {
		slog.Error("failed to move category", "id", id, "app_id", appID, "error", err)
		return nil, err
	}
	slog.Info("category moved", "id", id, "old_path", oldPath, "new_path", newPath)
	return s.repo.GetByID(ctx, appID, id)
}

func (s *service) Delete(ctx context.Context, appID, id int) error {
	if _, err := s.repo.GetByID(ctx, appID, id); err != nil {
		return err
	}

	children, err := s.repo.CountChildren(ctx, id)
	if err != nil {
		return err
	}
	if children > 0 {
		return ErrHasChildren
	}

	products, err := s.repo.CountProducts(ctx, id)
	if err != nil {
		return err
	}
	if products > 0 {
		return ErrHasProducts
	}

	if err := s.repo.Delete(ctx, appID, id); err != nil {
		slog.Error("failed to delete category", "id", id, "app_id", appID, "error", err)
		return err
	}
	slog.Info("category deleted", "id", id, "app_id", appID)
	return nil
}
