package ordergroup

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
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.GetByID)
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Patch("/status", h.UpdateStatus)
	})
	return r
}

func (h *Handler) getClaims(r *http.Request) (*auth.UserClaims, error) {
	claims, ok := auth.GetClaimsFromContext(r.Context())
	if !ok {
		return nil, errors.New("user not authenticated")
	}
	return claims, nil
}

func (h *Handler) renderError(w http.ResponseWriter, err error) {
	var validationErrs validator.ValidationErrors
	switch {
	case errors.As(err, &validationErrs):
		render.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrOrderGroupNotFound):
		render.Error(w, http.StatusNotFound, ErrOrderGroupNotFound.Error())
	case errors.Is(err, ErrOrderGroupInUse):
		render.Error(w, http.StatusConflict, ErrOrderGroupInUse.Error())
	default:
		render.Error(w, http.StatusInternalServerError, err.Error())
	}
}

// Create handles order group creation.
// @Summary Create a new order group
// @Description Create an order group (tab) under which multiple orders can be clubbed and settled together. A unique token is generated for the group.
// @Tags order-groups
// @Accept json
// @Produce json
// @Param body body CreateOrderGroupRequest true "Order group object"
// @Success 201 {object} render.Response{data=OrderGroup}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /order-groups [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req CreateOrderGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.Create(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusCreated, item)
}

// List handles listing all order groups.
// @Summary List all order groups
// @Description Get a summary list of order groups (orders_count only) for the authenticated app
// @Tags order-groups
// @Produce json
// @Param limit query int false "Max items to return (default 50, max 500)"
// @Param offset query int false "Items to skip (default 0)"
// @Param status query int false "Filter by status (1=open, 2=closed, 3=paid, 4=cancelled)"
// @Success 200 {object} render.Response{data=[]OrderGroup}
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /order-groups [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	p := platformhttp.ParsePagination(r)
	status := platformhttp.ParseStatusFilter(r)
	items, err := h.svc.List(r.Context(), claims.AppID, p.Limit, p.Offset, status)
	if err != nil {
		render.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	render.JSON(w, http.StatusOK, items)
}

// GetByID handles getting an order group by ID.
// @Summary Get order group by ID
// @Description Get a single order group with its member orders and the combined total
// @Tags order-groups
// @Produce json
// @Param id path int true "Order Group ID"
// @Success 200 {object} render.Response{data=OrderGroup}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /order-groups/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order group ID")
		return
	}

	item, err := h.svc.GetByID(r.Context(), claims.AppID, id)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Update handles updating an order group's label.
// @Summary Update order group by ID
// @Description Update the order group's label. Status is managed via /order-groups/{id}/status.
// @Tags order-groups
// @Accept json
// @Produce json
// @Param id path int true "Order Group ID"
// @Param body body UpdateOrderGroupRequest true "Order group object"
// @Success 200 {object} render.Response{data=OrderGroup}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /order-groups/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order group ID")
		return
	}

	var req UpdateOrderGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.Update(r.Context(), claims.AppID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// UpdateStatus handles setting an order group's status.
// @Summary Set order group status
// @Description Set the group status (1=open, 2=closed, 3=paid, 4=cancelled)
// @Tags order-groups
// @Accept json
// @Produce json
// @Param id path int true "Order Group ID"
// @Param body body UpdateOrderGroupStatusRequest true "Status object"
// @Success 200 {object} render.Response{data=OrderGroup}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /order-groups/{id}/status [patch]
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order group ID")
		return
	}

	var req UpdateOrderGroupStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.UpdateStatus(r.Context(), claims.AppID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Delete handles deleting an order group.
// @Summary Delete order group by ID
// @Description Delete an order group. Member orders are detached (kept as standalone orders), not deleted.
// @Tags order-groups
// @Produce json
// @Param id path int true "Order Group ID"
// @Success 204 "No Content"
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /order-groups/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order group ID")
		return
	}

	if err := h.svc.Delete(r.Context(), claims.AppID, id); err != nil {
		h.renderError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getIDParam(r *http.Request) (int, error) {
	return strconv.Atoi(chi.URLParam(r, "id"))
}
