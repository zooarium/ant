package category

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	platformhttp "ant/internal/platform/http"
	"ant/internal/platform/render"

	"keeper/pkg/auth"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	svc      Service
	validate *validator.Validate
}

func NewHandler(svc Service) *Handler {
	return &Handler{
		svc:      svc,
		validate: validator.New(),
	}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Put("/reorder", h.Reorder)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.GetByID)
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Get("/descendants", h.Descendants)
		r.Put("/move", h.Move)
	})
	return r
}

// PublicRoutes exposes the read-only category tree for the public order-intake
// page (mounted at /public/categories). Scoped by app id via the guest token.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Get("/{id}", h.GetByID)
	r.Get("/{id}/descendants", h.Descendants)
	return r
}

func (h *Handler) getClaims(r *http.Request) (*auth.UserClaims, error) {
	claims, ok := auth.GetClaimsFromContext(r.Context())
	if !ok {
		return nil, errors.New("user not authenticated")
	}
	return claims, nil
}

func (h *Handler) parseID(r *http.Request) (int, error) {
	return strconv.Atoi(chi.URLParam(r, "id"))
}

func (h *Handler) renderError(w http.ResponseWriter, err error) {
	var validationErrs validator.ValidationErrors
	switch {
	case errors.As(err, &validationErrs):
		render.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrCategoryNotFound):
		render.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrHasChildren), errors.Is(err, ErrHasProducts):
		render.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrParentNotFound),
		errors.Is(err, ErrParentInactive),
		errors.Is(err, ErrMoveToSelf),
		errors.Is(err, ErrMoveToDescendant),
		errors.Is(err, ErrDuplicateReorder):
		render.Error(w, http.StatusBadRequest, err.Error())
	default:
		render.Error(w, http.StatusInternalServerError, err.Error())
	}
}

// Create handles category creation.
// @Summary Create a new category
// @Description Create a category, optionally under a parent for hierarchical grouping.
// @Tags categories
// @Accept json
// @Produce json
// @Param body body CreateCategoryRequest true "Category object"
// @Success 201 {object} render.Response{data=Category}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /categories [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		render.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	item, err := h.svc.Create(r.Context(), claims.AppID, claims.DivisionID, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusCreated, item)
}

// List handles listing categories.
// @Summary List categories
// @Description List categories for the authenticated app. Filter by parent_id and status.
// @Tags categories
// @Produce json
// @Param parent_id query int false "Filter by parent ID"
// @Param status query int false "Filter by status (0 or 1)"
// @Param limit query int false "Max items to return (default 50, max 500)"
// @Param offset query int false "Items to skip (default 0)"
// @Success 200 {object} render.Response{data=[]Category}
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /categories [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var parentID *int
	if v := r.URL.Query().Get("parent_id"); v != "" {
		pid, err := strconv.Atoi(v)
		if err != nil {
			render.Error(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		parentID = &pid
	}

	p := platformhttp.ParsePagination(r)
	status := platformhttp.ParseStatusFilter(r)
	items, err := h.svc.List(r.Context(), claims.AppID, claims.DivisionID, parentID, status, p.Limit, p.Offset)
	if err != nil {
		render.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	render.JSON(w, http.StatusOK, items)
}

// GetByID handles getting a category by ID.
// @Summary Get category by ID
// @Description Get a single category with its display hierarchy.
// @Tags categories
// @Produce json
// @Param id path int true "Category ID"
// @Success 200 {object} render.Response{data=Category}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Security Bearer
// @Router /categories/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.parseID(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	item, err := h.svc.GetByID(r.Context(), claims.AppID, claims.DivisionID, id)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Descendants handles listing a category's subtree.
// @Summary Get all descendants of a category
// @Description Returns all categories in the subtree rooted at the given category.
// @Tags categories
// @Produce json
// @Param id path int true "Category ID"
// @Success 200 {object} render.Response{data=[]Category}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Security Bearer
// @Router /categories/{id}/descendants [get]
func (h *Handler) Descendants(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.parseID(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	items, err := h.svc.Descendants(r.Context(), claims.AppID, claims.DivisionID, id)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, items)
}

// Update handles updating a category.
// @Summary Update category by ID
// @Description Update a category's name or status.
// @Tags categories
// @Accept json
// @Produce json
// @Param id path int true "Category ID"
// @Param body body UpdateCategoryRequest true "Category object"
// @Success 200 {object} render.Response{data=Category}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /categories/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.parseID(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	var req UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		render.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	item, err := h.svc.Update(r.Context(), claims.AppID, claims.DivisionID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Move handles reparenting a category.
// @Summary Move a category to a new parent
// @Description Move a category (and its subtree) to a different parent. Null parent_id promotes to root.
// @Tags categories
// @Accept json
// @Produce json
// @Param id path int true "Category ID"
// @Param body body MoveCategoryRequest true "New parent"
// @Success 200 {object} render.Response{data=Category}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /categories/{id}/move [put]
func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.parseID(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	var req MoveCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.Move(r.Context(), claims.AppID, claims.DivisionID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Reorder handles bulk-setting category display positions.
// @Summary Reorder categories
// @Description Atomically set the display position (ord) of many categories at once. Categories list in ord ASC, id ASC.
// @Tags categories
// @Accept json
// @Produce json
// @Param body body ReorderRequest true "Category positions"
// @Success 204 "No Content"
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /categories/reorder [put]
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		render.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.Reorder(r.Context(), claims.AppID, claims.DivisionID, req); err != nil {
		h.renderError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Delete handles deleting a category.
// @Summary Delete category by ID
// @Description Delete a category. Blocked if it has children or assigned products.
// @Tags categories
// @Produce json
// @Param id path int true "Category ID"
// @Success 204 "No Content"
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 409 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /categories/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.parseID(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid category ID")
		return
	}

	if err := h.svc.Delete(r.Context(), claims.AppID, claims.DivisionID, id); err != nil {
		h.renderError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
