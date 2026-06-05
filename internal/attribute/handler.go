package attribute

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
	case errors.Is(err, ErrAttributeNotFound):
		render.Error(w, http.StatusNotFound, ErrAttributeNotFound.Error())
	case errors.Is(err, ErrOptionNotFound):
		render.Error(w, http.StatusNotFound, ErrOptionNotFound.Error())
	case errors.Is(err, ErrAttributeInUse):
		render.Error(w, http.StatusConflict, ErrAttributeInUse.Error())
	case errors.Is(err, ErrDuplicateOptionValue):
		render.Error(w, http.StatusBadRequest, ErrDuplicateOptionValue.Error())
	default:
		render.Error(w, http.StatusInternalServerError, err.Error())
	}
}

// Create handles attribute creation.
// @Summary Create a new attribute
// @Description Create a new attribute with optional options
// @Tags attributes
// @Accept json
// @Produce json
// @Param body body CreateAttributeRequest true "Attribute object"
// @Success 201 {object} render.Response{data=Attribute}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /attributes [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req CreateAttributeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.Create(r.Context(), claims.AppID, claims.UserID, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusCreated, item)
}

// List handles listing all attributes.
// @Summary List all attributes
// @Description Get a list of all attributes for the authenticated app
// @Tags attributes
// @Produce json
// @Param limit query int false "Max items to return (default 50, max 500)"
// @Param offset query int false "Items to skip (default 0)"
// @Param status query int false "Filter by status (0 or 1)"
// @Success 200 {object} render.Response{data=[]Attribute}
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /attributes [get]
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

// GetByID handles getting an attribute by ID.
// @Summary Get attribute by ID
// @Description Get a single attribute with its options
// @Tags attributes
// @Produce json
// @Param id path int true "Attribute ID"
// @Success 200 {object} render.Response{data=Attribute}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /attributes/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid attribute ID")
		return
	}

	item, err := h.svc.GetByID(r.Context(), claims.AppID, claims.UserID, id)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Update handles updating an attribute.
// @Summary Update attribute by ID
// @Description Atomically update an attribute and sync its options in one call: options with an id are updated, ones without are created, existing options missing from the payload are deleted.
// @Tags attributes
// @Accept json
// @Produce json
// @Param id path int true "Attribute ID"
// @Param body body UpdateAttributeRequest true "Attribute object"
// @Success 200 {object} render.Response{data=Attribute}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /attributes/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid attribute ID")
		return
	}

	var req UpdateAttributeRequest
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

// Delete handles deleting an attribute.
// @Summary Delete attribute by ID
// @Description Delete an attribute and its options. Blocked if assigned to any product.
// @Tags attributes
// @Produce json
// @Param id path int true "Attribute ID"
// @Success 204 "No Content"
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 404 {object} render.Response
// @Failure 409 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /attributes/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	id, err := h.getIDParam(r, "id")
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid attribute ID")
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
