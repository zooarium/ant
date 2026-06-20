package category

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ant/ent"
	entcategory "ant/ent/category"
	"ant/ent/predicate"
	entproduct "ant/ent/product"
)

type categoryRepository struct {
	client *ent.Client
}

// NewRepository creates a new category repository.
func NewRepository(client *ent.Client) *categoryRepository {
	return &categoryRepository{client: client}
}

// Create inserts a category. parentPath is the parent's path ("/" for root);
// path and depth are computed after the row is saved to obtain the new id.
func (r *categoryRepository) Create(ctx context.Context, c Category, parentPath string) (*Category, error) {
	q := r.client.Category.Create().
		SetAppID(c.AppID).
		SetDivisionID(c.DivisionID).
		SetName(c.Name).
		SetPath("").
		SetDepth(0).
		SetStatus(c.Status)
	if c.ParentID != nil {
		q = q.SetParentID(*c.ParentID)
	}

	created, err := q.Save(ctx)
	if err != nil {
		slog.Error("database error: failed to create category", "name", c.Name, "error", err)
		return nil, err
	}

	finalPath := fmt.Sprintf("%s%d/", parentPath, created.ID)
	finalDepth := int8(strings.Count(finalPath, "/") - 2)

	updated, err := r.client.Category.UpdateOneID(created.ID).
		SetPath(finalPath).
		SetDepth(finalDepth).
		Save(ctx)
	if err != nil {
		slog.Error("database error: failed to set path for category", "id", created.ID, "error", err)
		_ = r.client.Category.DeleteOneID(created.ID).Exec(ctx)
		return nil, err
	}

	c2 := r.mapToModel(updated)
	if err := r.decorate(ctx, c.AppID, c.DivisionID, []*Category{c2}); err != nil {
		return nil, err
	}
	return c2, nil
}

// GetByID returns a category scoped to an app and division. appID=0 bypasses the
// app filter; divisionID=0 bypasses the division filter.
func (r *categoryRepository) GetByID(ctx context.Context, appID, divisionID, id int) (*Category, error) {
	q := r.client.Category.Query().Where(entcategory.IDEQ(id))
	if appID != 0 {
		q = q.Where(entcategory.AppIDEQ(appID))
	}
	if divisionID != 0 {
		q = q.Where(entcategory.DivisionIDEQ(divisionID))
	}
	c, err := q.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCategoryNotFound
		}
		slog.Error("database error: failed to get category by id", "id", id, "error", err)
		return nil, err
	}
	model := r.mapToModel(c)
	if err := r.decorate(ctx, appID, divisionID, []*Category{model}); err != nil {
		return nil, err
	}
	return model, nil
}

// List returns categories scoped to an app and division. If parentID is nil, returns all.
func (r *categoryRepository) List(ctx context.Context, appID, divisionID int, parentID *int, status *int8, limit, offset int) ([]*Category, error) {
	q := r.client.Category.Query()
	if appID != 0 {
		q = q.Where(entcategory.AppIDEQ(appID))
	}
	if divisionID != 0 {
		q = q.Where(entcategory.DivisionIDEQ(divisionID))
	}
	if parentID != nil {
		q = q.Where(entcategory.ParentIDEQ(*parentID))
	}
	if status != nil {
		q = q.Where(entcategory.StatusEQ(*status))
	}
	rows, err := q.
		Order(ent.Asc(entcategory.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		slog.Error("database error: failed to list categories", "app_id", appID, "error", err)
		return nil, err
	}
	result := make([]*Category, len(rows))
	for i, c := range rows {
		result[i] = r.mapToModel(c)
	}
	if err := r.decorate(ctx, appID, divisionID, result); err != nil {
		return nil, err
	}
	return result, nil
}

// Descendants returns the subtree rooted at path, excluding the node itself.
func (r *categoryRepository) Descendants(ctx context.Context, appID, divisionID int, path string) ([]*Category, error) {
	predicates := []predicate.Category{
		entcategory.PathHasPrefix(path),
		entcategory.PathNEQ(path),
	}
	if appID != 0 {
		predicates = append(predicates, entcategory.AppIDEQ(appID))
	}
	if divisionID != 0 {
		predicates = append(predicates, entcategory.DivisionIDEQ(divisionID))
	}
	rows, err := r.client.Category.Query().
		Where(predicates...).
		Order(ent.Asc(entcategory.FieldPath)).
		All(ctx)
	if err != nil {
		slog.Error("database error: failed to get category descendants", "path", path, "error", err)
		return nil, err
	}
	result := make([]*Category, len(rows))
	for i, c := range rows {
		result[i] = r.mapToModel(c)
	}
	if err := r.decorate(ctx, appID, divisionID, result); err != nil {
		return nil, err
	}
	return result, nil
}

// Update sets name and status of a category.
func (r *categoryRepository) Update(ctx context.Context, appID, divisionID, id int, c *Category) (*Category, error) {
	q := r.client.Category.Update().Where(entcategory.IDEQ(id))
	if appID != 0 {
		q = q.Where(entcategory.AppIDEQ(appID))
	}
	if divisionID != 0 {
		q = q.Where(entcategory.DivisionIDEQ(divisionID))
	}
	count, err := q.
		SetName(c.Name).
		SetStatus(c.Status).
		Save(ctx)
	if err != nil {
		slog.Error("database error: failed to update category", "id", id, "error", err)
		return nil, err
	}
	if count == 0 {
		return nil, ErrCategoryNotFound
	}
	return r.GetByID(ctx, appID, divisionID, id)
}

// Move atomically reparents a category and cascades the path/depth update
// across the node and all descendants inside a single transaction.
func (r *categoryRepository) Move(ctx context.Context, appID, divisionID, id int, newParentID *int, oldPath, newPath string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		slog.Error("database error: failed to begin transaction for category move", "id", id, "error", err)
		return fmt.Errorf("begin tx: %w", err)
	}

	if err := r.moveTx(ctx, tx, appID, divisionID, id, newParentID, oldPath, newPath); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			slog.Error("database error: failed to rollback category move", "id", id, "error", rerr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		slog.Error("database error: failed to commit category move", "id", id, "error", err)
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *categoryRepository) moveTx(ctx context.Context, tx *ent.Tx, appID, divisionID, id int, newParentID *int, oldPath, newPath string) error {
	cascade := []predicate.Category{entcategory.PathHasPrefix(oldPath)}
	if appID != 0 {
		cascade = append(cascade, entcategory.AppIDEQ(appID))
	}
	if divisionID != 0 {
		cascade = append(cascade, entcategory.DivisionIDEQ(divisionID))
	}
	affected, err := tx.Category.Query().
		Where(cascade...).
		All(ctx)
	if err != nil {
		slog.Error("database error: failed to fetch categories for path cascade", "old_path", oldPath, "error", err)
		return err
	}

	for _, c := range affected {
		updatedPath := newPath + c.Path[len(oldPath):]
		updatedDepth := int8(strings.Count(updatedPath, "/") - 2)
		if _, err := tx.Category.UpdateOneID(c.ID).
			SetPath(updatedPath).
			SetDepth(updatedDepth).
			Save(ctx); err != nil {
			slog.Error("database error: failed to cascade category path", "id", c.ID, "error", err)
			return err
		}
	}

	u := tx.Category.UpdateOneID(id)
	if newParentID != nil {
		u = u.SetParentID(*newParentID)
	} else {
		u = u.ClearParentID()
	}
	if _, err := u.Save(ctx); err != nil {
		slog.Error("database error: failed to move category", "id", id, "error", err)
		return err
	}
	return nil
}

// CountChildren returns the number of direct children of a category.
func (r *categoryRepository) CountChildren(ctx context.Context, id int) (int, error) {
	return r.client.Category.Query().
		Where(entcategory.ParentIDEQ(id)).
		Count(ctx)
}

// CountProducts returns the number of products assigned to a category.
func (r *categoryRepository) CountProducts(ctx context.Context, id int) (int, error) {
	return r.client.Product.Query().
		Where(entproduct.CategoryIDEQ(id)).
		Count(ctx)
}

// Delete removes a category scoped to an app and division.
func (r *categoryRepository) Delete(ctx context.Context, appID, divisionID, id int) error {
	q := r.client.Category.Delete().Where(entcategory.IDEQ(id))
	if appID != 0 {
		q = q.Where(entcategory.AppIDEQ(appID))
	}
	if divisionID != 0 {
		q = q.Where(entcategory.DivisionIDEQ(divisionID))
	}
	count, err := q.Exec(ctx)
	if err != nil {
		slog.Error("database error: failed to delete category", "id", id, "error", err)
		return err
	}
	if count == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

// decorate fills the Display field of each category by resolving ancestor
// names in a single batched query (no N+1).
func (r *categoryRepository) decorate(ctx context.Context, appID, divisionID int, cats []*Category) error {
	if len(cats) == 0 {
		return nil
	}

	ancestorIDs := make(map[int]struct{})
	for _, c := range cats {
		ids := ParsePathIDs(c.Path)
		if len(ids) <= 1 {
			continue
		}
		for _, id := range ids[:len(ids)-1] {
			ancestorIDs[id] = struct{}{}
		}
	}

	nameByID := make(map[int]string, len(ancestorIDs))
	if len(ancestorIDs) > 0 {
		idList := make([]int, 0, len(ancestorIDs))
		for id := range ancestorIDs {
			idList = append(idList, id)
		}
		q := r.client.Category.Query().Where(entcategory.IDIn(idList...))
		if appID != 0 {
			q = q.Where(entcategory.AppIDEQ(appID))
		}
		if divisionID != 0 {
			q = q.Where(entcategory.DivisionIDEQ(divisionID))
		}
		rows, err := q.
			Select(entcategory.FieldID, entcategory.FieldName).
			All(ctx)
		if err != nil {
			slog.Error("database error: failed to load ancestor names", "error", err)
			return err
		}
		for _, row := range rows {
			nameByID[row.ID] = row.Name
		}
	}

	for _, c := range cats {
		ids := ParsePathIDs(c.Path)
		var ancestors []string
		if len(ids) > 1 {
			for _, id := range ids[:len(ids)-1] {
				if n, ok := nameByID[id]; ok {
					ancestors = append(ancestors, n)
				}
			}
		}
		c.Display = BuildDisplay(c.Name, ancestors)
	}
	return nil
}

func (r *categoryRepository) mapToModel(c *ent.Category) *Category {
	return &Category{
		ID:         c.ID,
		AppID:      c.AppID,
		DivisionID: c.DivisionID,
		ParentID:   c.ParentID,
		Name:       c.Name,
		Path:       c.Path,
		Depth:      c.Depth,
		Status:     c.Status,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}
