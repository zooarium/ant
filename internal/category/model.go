package category

import (
	"strconv"
	"strings"
	"time"
)

// Category is the domain model for a product category. Path is the
// materialized path of ancestor+self ids (e.g. "/5/8/", self last); Display
// renders the name followed by its ancestor hierarchy in parentheses
// (e.g. "Laptops (Electronics > Computers)").
type Category struct {
	ID         int       `json:"id"`
	AppID      int       `json:"app_id"`
	DivisionID int       `json:"division_id"`
	ParentID   *int      `json:"parent_id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Depth      int8      `json:"depth"`
	Status     int8      `json:"status"`
	Display    string    `json:"display"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateCategoryRequest is the payload for creating a category.
type CreateCategoryRequest struct {
	ParentID *int   `json:"parent_id"`
	Name     string `json:"name" validate:"required,max=100"`
}

// UpdateCategoryRequest is the payload for updating a category's name/status.
type UpdateCategoryRequest struct {
	Name   *string `json:"name"   validate:"omitempty,max=100"`
	Status *int8   `json:"status" validate:"omitempty,oneof=0 1"`
}

// MoveCategoryRequest is the payload for reparenting a category. A nil
// parent_id promotes the category to a root.
type MoveCategoryRequest struct {
	ParentID *int `json:"parent_id"`
}

// ParsePathIDs returns the ordered ids embedded in a materialized path
// ("/5/8/" -> [5, 8]); the last id is the node itself, the rest its ancestors.
func ParsePathIDs(path string) []int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			ids = append(ids, n)
		}
	}
	return ids
}

// BuildDisplay renders "Name (A > B > C)" from a name and its ancestor names
// (root-first, self excluded). With no ancestors it returns the bare name.
func BuildDisplay(name string, ancestors []string) string {
	if len(ancestors) == 0 {
		return name
	}
	return name + " (" + strings.Join(ancestors, " > ") + ")"
}
