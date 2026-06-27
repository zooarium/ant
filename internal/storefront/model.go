package storefront

import (
	"time"

	"ant/ent/schema"
	"ant/internal/product"
)

// The JSON value types are shared with the ent schema so the API shape and the
// stored shape stay in lockstep (single source of truth). Validation of their
// contents lives in the service layer.
type (
	Assessment   = schema.Assessment
	Review       = schema.Review
	GalleryImage = schema.GalleryImage
	FoodTag      = schema.FoodTag
)

// Storefront is the domain model for a tenant's public presentation config.
// Exactly one exists per (app_id, division_id).
type Storefront struct {
	ID          int            `json:"id"`
	AppID       int            `json:"app_id"`
	DivisionID  int            `json:"division_id"`
	HeroImage   string         `json:"hero_image"`
	Assessments []Assessment   `json:"assessments"`
	Gallery     []GalleryImage `json:"gallery"`
	FoodTags    []FoodTag      `json:"food_tags"`
	Status      int8           `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// PublicStorefront is the public read shape: the storefront config plus the
// tenant's active product catalog (the menu). Products are returned unpaginated
// but bounded by PUBLIC_STOREFRONT.MAX_PRODUCTS; each carries its category ref
// (with display) so the UI can group the menu client-side without a separate
// categories array.
type PublicStorefront struct {
	*Storefront
	Products []product.Product `json:"products"`
}

// UpsertStorefrontRequest is the payload for creating/replacing the storefront.
// The whole object is replaced on save: gallery/tag/assessment add/edit/delete
// is performed client-side by sending the full desired arrays. Status is
// optional (defaults to active on first save, unchanged thereafter).
type UpsertStorefrontRequest struct {
	HeroImage   string         `json:"hero_image"`
	Assessments []Assessment   `json:"assessments"`
	Gallery     []GalleryImage `json:"gallery"`
	FoodTags    []FoodTag      `json:"food_tags"`
	Status      *int8          `json:"status"`
}
