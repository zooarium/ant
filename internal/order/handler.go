package order

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	platformhttp "ant/internal/platform/http"
	"ant/internal/platform/render"

	"keeper/pkg/auth"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	svc      Service
	validate *validator.Validate
	// maxPublicOrders / publicOrderWindow cap public order creation per device
	// (or IP). Sourced from config; passed through to the service on each
	// public create. Zero maxPublicOrders disables the cap.
	maxPublicOrders   int
	publicOrderWindow time.Duration
}

func NewHandler(svc Service, maxPublicOrders int, publicOrderWindow time.Duration) *Handler {
	return &Handler{
		svc:               svc,
		validate:          validator.New(),
		maxPublicOrders:   maxPublicOrders,
		publicOrderWindow: publicOrderWindow,
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
		r.Patch("/group", h.SetGroup)
	})
	return r
}

// PublicRoutes returns the order routes exposed under the /public prefix.
// Mounted at /public/orders by the router, so POST / becomes
// POST /public/orders. Guest-token callers create pending orders here; the
// write route is wrapped with the supplied captcha middleware.
func (h *Handler) PublicRoutes(captcha func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.With(captcha).Post("/", h.CreatePublic)
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
		errors.Is(err, ErrOrderGroupInvalid),
		errors.Is(err, ErrInvalidCustomerContact),
		errors.Is(err, ErrTaxPercentNotAllowed):
		render.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrPublicOrderLimit):
		render.Error(w, http.StatusTooManyRequests, ErrPublicOrderLimit.Error())
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

	item, err := h.svc.Create(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, clientIP(r), req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusCreated, item)
}

// CreatePublic handles public order creation from the order-intake page.
// @Summary Create an order (public)
// @Description Creates a pending order on behalf of a guest-token caller, optionally attaching it to an existing group (group_id) or auto-minting a new one. The order is always created pending — confirmation and payment are done at reception. Protected by reCAPTCHA, a hidden honeypot field, and a per-device volume cap.
// @Tags Public
// @Accept json
// @Produce json
// @Param body body CreatePublicOrderRequest true "Order object"
// @Success 201 {object} render.Response{data=Order}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 403 {object} render.Response
// @Failure 429 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /public/orders [post]
func (h *Handler) CreatePublic(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req CreatePublicOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Honeypot: a real client never fills this hidden field. Silently accept
	// (200, no order) so a bot cannot tell it was caught and tune around it.
	if req.Honeypot != "" {
		slog.Warn("public order honeypot tripped", "app_id", claims.AppID, "division_id", claims.DivisionID, "ip", clientIP(r))
		render.JSON(w, http.StatusOK, nil)
		return
	}

	item, err := h.svc.CreatePublic(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, clientIP(r), req, h.maxPublicOrders, h.publicOrderWindow)
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
// @Param status query int false "Filter by status (1=pending, 2=confirmed, 3=completed, 4=cancelled, 5=paid)"
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
	items, err := h.svc.List(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, p.Limit, p.Offset, status)
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

	item, err := h.svc.GetByID(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, id)
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

	item, err := h.svc.Update(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// UpdateStatus handles setting an order's status.
// @Summary Set order status
// @Description Set the order status (1=pending, 2=confirmed, 3=completed, 4=cancelled, 5=paid)
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

	item, err := h.svc.UpdateStatus(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, id, req)
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

	if err := h.svc.Delete(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, id); err != nil {
		h.renderError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetGroup attaches an order to a group or detaches it.
// @Summary Attach/detach order to a group
// @Description Set group_id to attach the order to an order group (tab), or omit/null to detach it.
// @Tags orders
// @Accept json
// @Produce json
// @Param id path int true "Order ID"
// @Param body body SetOrderGroupRequest true "Group assignment"
// @Success 200 {object} render.Response{data=Order}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /orders/{id}/group [patch]
func (h *Handler) SetGroup(w http.ResponseWriter, r *http.Request) {
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

	var req SetOrderGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.SetGroup(r.Context(), claims.AppID, claims.UserID, claims.DivisionID, id, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

func (h *Handler) getIDParam(r *http.Request, name string) (int, error) {
	return strconv.Atoi(chi.URLParam(r, name))
}

// clientIP extracts the originating client IP, preferring proxy headers
// (X-Forwarded-For first hop, then X-Real-IP) and falling back to the raw
// connection address. Behind a trusted proxy the headers are authoritative;
// the value is stored as a best-effort audit signal, not for access control.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
