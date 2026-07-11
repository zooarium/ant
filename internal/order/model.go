package order

import (
	"time"
)

// Order status values.
const (
	StatusPending   int8 = 1
	StatusConfirmed int8 = 2
	StatusCompleted int8 = 3
	StatusCancelled int8 = 4
	StatusPaid      int8 = 5
)

type Order struct {
	ID              int       `json:"id"`
	AppID           int       `json:"app_id"`
	UserID          int       `json:"user_id"`
	DivisionID      int       `json:"division_id"`
	GroupID         int       `json:"group_id"`
	GroupToken      string    `json:"group_token,omitempty"`
	CustomerName    string    `json:"customer_name"`
	CustomerContact string    `json:"customer_contact"`
	Status          int8      `json:"status"`
	OrderedAt       time.Time `json:"ordered_at"`
	// TaxPercent is the tax rate applied to the order as a percentage
	// (e.g. 18.5 = 18.5%). Range 0–100.
	TaxPercent float64 `json:"tax_percent"`
	// IPAddress is the client IP captured server-side at creation. Read-only.
	IPAddress string `json:"ip_address,omitempty"`
	// DeviceID is the client-supplied device identifier used for returning-
	// customer recognition on the public order-intake page.
	DeviceID      string `json:"device_id,omitempty"`
	ProductsCount int    `json:"products_count"`
	// Total is the pre-tax order amount: sum over items of (base price + chosen
	// option deltas) * quantity. Persisted as a denormalized column and
	// maintained on every item change, so it is populated on both list and
	// detail reads.
	Total     float64     `json:"total"`
	Products  []OrderItem `json:"products,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// OrderItem is a denormalized snapshot of a product at the moment it was
// added to the order. It does not depend on the product table.
type OrderItem struct {
	ID          int                  `json:"id"`
	ProductID   int                  `json:"product_id"`
	ProductName string               `json:"product_name"`
	Price       float64              `json:"price"`
	Quantity    int                  `json:"quantity"`
	Attributes  []OrderItemAttribute `json:"attributes"`
	// LineTotal is (Price + sum of attribute PriceDelta) * Quantity.
	LineTotal float64 `json:"line_total"`
}

type OrderItemAttribute struct {
	AttributeID   int     `json:"attribute_id"`
	AttributeName string  `json:"attribute_name"`
	OptionID      int     `json:"option_id"`
	OptionValue   string  `json:"option_value"`
	PriceDelta    float64 `json:"price_delta"`
}

type OrderItemAttributeRequest struct {
	AttributeID int `json:"attribute_id" validate:"required"`
	OptionID    int `json:"option_id" validate:"required"`
}

type OrderItemRequest struct {
	ProductID  int                         `json:"product_id" validate:"required"`
	Quantity   int                         `json:"quantity" validate:"required,min=1"`
	Attributes []OrderItemAttributeRequest `json:"attributes" validate:"omitempty,dive"`
}

type CreateOrderRequest struct {
	CustomerName    string `json:"customer_name" validate:"required,max=100"`
	CustomerContact string `json:"customer_contact" validate:"required,min=7,max=20"`
	Status          *int8  `json:"status" validate:"omitempty,oneof=1 2 3 4 5"`
	// TaxPercent is the order tax rate as a percentage (0–100). Defaults to 0
	// when omitted.
	TaxPercent *float64 `json:"tax_percent" validate:"omitempty,min=0,max=100"`
	// OrderedAt sets the business order date; defaults to now when omitted.
	OrderedAt *time.Time `json:"ordered_at" validate:"omitempty"`
	// GroupID attaches the order to an existing group (tab). When omitted, a
	// new group is minted in the same transaction and attached automatically;
	// its token is returned in the order's group_token so the UI can reuse it
	// to attach later orders to the same tab.
	GroupID *int `json:"group_id" validate:"omitempty,min=1"`
	// GroupLabel optionally names the tab when a new group is auto-created
	// (ignored when group_id is supplied).
	GroupLabel string `json:"group_label" validate:"omitempty,max=100"`
	// DeviceID identifies the client device for returning-customer recognition
	// on the public order-intake page. Optional; the IP is captured server-side.
	DeviceID string             `json:"device_id" validate:"omitempty,max=64"`
	Products []OrderItemRequest `json:"products" validate:"required,min=1,dive"`
}

// CreatePublicOrderRequest is the payload for the public order-intake endpoint.
// It embeds CreateOrderRequest (reusing all of its rules), except tax_percent,
// which is rejected with 400 — tax is never guest-controlled. It adds a honeypot
// field: a legitimate client leaves Honeypot empty, so any non-empty value
// marks the request as a bot and is silently dropped. The JSON name "website"
// is generic so it blends in as a normal hidden form field.
type CreatePublicOrderRequest struct {
	CreateOrderRequest
	Honeypot string `json:"website" validate:"max=0"`
}

// UpdateOrderRequest atomically replaces the order's customer details and
// syncs its items in one call. Items are synced by id: an item with an id
// keeps its product/attribute snapshot and only its quantity is editable; an
// item without an id is added (product_id + attributes required); existing
// items missing from the payload are deleted. Status is managed separately
// via /orders/{id}/status.
type UpdateOrderRequest struct {
	CustomerName    string `json:"customer_name" validate:"required,max=100"`
	CustomerContact string `json:"customer_contact" validate:"required,min=7,max=20"`
	// TaxPercent is the order tax rate as a percentage (0–100). When omitted the
	// existing value is preserved.
	TaxPercent *float64               `json:"tax_percent" validate:"omitempty,min=0,max=100"`
	OrderedAt  *time.Time             `json:"ordered_at" validate:"omitempty"`
	Products   []SyncOrderItemRequest `json:"products" validate:"required,min=1,dive"`
}

// SetOrderGroupRequest moves the order to a different group. Every order must
// belong to a group, so group_id is required (no detach).
type SetOrderGroupRequest struct {
	GroupID *int `json:"group_id" validate:"required,min=1"`
}

// SyncOrderItemRequest is one item in an order update. Exactly one of id
// (existing item) or product_id (new item) must be set.
type SyncOrderItemRequest struct {
	ID         int                         `json:"id" validate:"omitempty,min=1"`
	ProductID  int                         `json:"product_id" validate:"omitempty,min=1"`
	Quantity   int                         `json:"quantity" validate:"required,min=1"`
	Attributes []OrderItemAttributeRequest `json:"attributes" validate:"omitempty,dive"`
}

type UpdateOrderStatusRequest struct {
	Status int8 `json:"status" validate:"required,oneof=1 2 3 4 5"`
}
