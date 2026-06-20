package storefront

import (
	"encoding/json"
	"errors"
	"net/http"

	"ant/internal/platform/render"

	"keeper/pkg/auth"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Routes are the authenticated admin routes (primary port).
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Get)
	r.Put("/", h.Upsert)
	return r
}

// PublicRoutes exposes the read-only storefront for the public page, mounted
// at /public/storefront on the guest-token listener. Scoped by the guest
// token's app_id/division_id claims.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Get)
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
	switch {
	case errors.Is(err, ErrInvalid):
		render.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrStorefrontNotFound):
		render.Error(w, http.StatusNotFound, err.Error())
	default:
		render.Error(w, http.StatusInternalServerError, err.Error())
	}
}

// Get handles reading the storefront.
// @Summary Get the storefront
// @Description Get the tenant's storefront config (branding, gallery, food tags, platform assessments). Returns an empty active storefront if none has been saved yet.
// @Tags storefront
// @Produce json
// @Success 200 {object} render.Response{data=Storefront}
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /storefront [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	item, err := h.svc.Get(r.Context(), claims.AppID, claims.DivisionID)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}

// Upsert handles creating/replacing the storefront.
// @Summary Create or replace the storefront
// @Description Replace the whole storefront in one save. Gallery, food-tag, and assessment add/edit/delete is done by sending the full desired arrays.
// @Tags storefront
// @Accept json
// @Produce json
// @Param body body UpsertStorefrontRequest true "Storefront object"
// @Success 200 {object} render.Response{data=Storefront}
// @Failure 400 {object} render.Response
// @Failure 401 {object} render.Response
// @Failure 500 {object} render.Response
// @Security Bearer
// @Router /storefront [put]
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	claims, err := h.getClaims(r)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req UpsertStorefrontRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.svc.Upsert(r.Context(), claims.AppID, claims.DivisionID, req)
	if err != nil {
		h.renderError(w, err)
		return
	}

	render.JSON(w, http.StatusOK, item)
}
