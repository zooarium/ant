package order

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
	case errors.Is(err, ErrOrderNotFound):
		render.Error(w, http.StatusNotFound, ErrOrderNotFound.Error())
	case errors.Is(err, ErrOrderItemNotFound):
		render.Error(w, http.StatusNotFound, ErrOrderItemNotFound.Error())
	case errors.Is(err, ErrProductInvalid),
		errors.Is(err, ErrProductInactive),
		errors.Is(err, ErrAttributeNotAssigned),
		errors.Is(err, ErrInvalidOption),
		errors.Is(err, ErrMandatoryAttributeMissing),
		errors.Is(err, ErrDuplicateItemAttribute),
		errors.Is(err, ErrOrderItemImmutable),
		errors.Is(err, ErrInvalidOrderItem),
		errors.Is(err, ErrDuplicateOrderItem),
		errors.Is(err, ErrInvalidCustomerContact):
		render.Error(w, http.StatusBadRequest, err.Error())
	default:
		render.Error(w, http.StatusInternalServerError, err.Error())
	}
}

// Create handles order creation.
// @Summary Create a new order
// @Description Create an order with one or more items. Items snapshot the product name, price and chosen attribute values; only active products can be added.
// @Tags orders
// @Accept json
// @Produce json
// @Param body body CreateOrderRequest true "Order object"
// @Success 201 {object} render.Response{data=Order}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req CreateOrderRequest
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

// List handles listing all orders.
// @Summary List all orders
// @Description Get a summary list of orders (no items, products_count only) for the authenticated app
// @Tags orders
// @Produce json
// @Param limit query int false "Max items to return (default 50, max 500)"
// @Param offset query int false "Items to skip (default 0)"
// @Param status query int false "Filter by status (1=pending, 2=confirmed, 3=completed, 4=cancelled)"
// @Success 200 {object} render.Response{data=[]Order}
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	p := platformhttp.ParsePagination(r)
	status := platformhttp.ParseStatusFilter(r)
	items, err := h.svc.List(r.Context(), claims.AppID, claims.UserID, p.Limit, p.Offset, status)
	if err != nil {
		render.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	render.JSON(w, http.StatusOK, items)
}

// GetByID handles getting an order by ID.
// @Summary Get order by ID
// @Description Get a single order with its items
// @Tags orders
// @Produce json
// @Param id path int true "Order ID"
// @Success 200 {object} render.Response{data=Order}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	item, err := h.svc.GetByID(r.Context(), claims.AppID, claims.UserID, id)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Update handles atomically updating an order.
// @Summary Update order by ID
// @Description Atomically update customer details and sync items in one call. Items with an id update quantity only (product/attributes immutable); items without an id are added; existing items missing from the payload are deleted. Status is managed via /orders/{id}/status.
// @Tags orders
// @Accept json
// @Produce json
// @Param id path int true "Order ID"
// @Param body body UpdateOrderRequest true "Order object"
// @Success 200 {object} render.Response{data=Order}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	var req UpdateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.Update(r.Context(), claims.AppID, claims.UserID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// UpdateStatus handles setting an order's status.
// @Summary Set order status
// @Description Set the order status (1=pending, 2=confirmed, 3=completed, 4=cancelled)
// @Tags orders
// @Accept json
// @Produce json
// @Param id path int true "Order ID"
// @Param body body UpdateOrderStatusRequest true "Status object"
// @Success 200 {object} render.Response{data=Order}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders/{id}/status [patch]
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	var req UpdateOrderStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.UpdateStatus(r.Context(), claims.AppID, claims.UserID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Delete handles deleting an order.
// @Summary Delete order by ID
// @Description Delete an order and all its items
// @Tags orders
// @Produce json
// @Param id path int true "Order ID"
// @Success 204 "No Content"
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	if err := h.svc.Delete(r.Context(), claims.AppID, claims.UserID, id); err != nil {
		h.renderError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getIDParam(r *http.Request, name string) (int, error) {
	return strconv.Atoi(chi.URLParam(r, name))
}
