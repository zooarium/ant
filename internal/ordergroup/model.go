package ordergroup

import (
	"time"

	"ant/pkg/keeper"
)

// Order group (tab) status values.
const (
	StatusOpen      int8 = 1
	StatusClosed    int8 = 2
	StatusPaid      int8 = 3
	StatusCancelled int8 = 4
)

type OrderGroup struct {
	ID         int    `json:"id"`
	AppID      int    `json:"app_id"`
	UserID     int    `json:"user_id"`
	DivisionID int    `json:"division_id"`
	Token      string `json:"token"`
	Label      string `json:"label"`
	Status     int8   `json:"status"`
	// OrdersCount is the number of orders in the group.
	OrdersCount int `json:"orders_count"`
	// Total is the sum of all member orders' totals. Populated on detail reads.
	Total float64 `json:"total"`
	// Orders is the member order summary list. Populated on detail reads.
	Orders    []OrderSummary `json:"orders,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	// App is the tenant's public profile (name, contact), enriched from keeper
	// on detail reads. Nil when keeper is unreachable — never blocks the read.
	App *keeper.AppProfile `json:"app,omitempty"`
}

// OrderSummary is a lightweight view of a member order within a group. Items
// are the snapshotted products (already loaded to compute Total), carried so
// the public tab/history views can show what was ordered without a per-order
// detail fetch.
type OrderSummary struct {
	ID           int         `json:"id"`
	CustomerName string      `json:"customer_name"`
	Status       int8        `json:"status"`
	OrderedAt    time.Time   `json:"ordered_at"`
	Total        float64     `json:"total"`
	Products     []OrderItem `json:"products,omitempty"`
}

// OrderItem is a snapshotted product line within an order summary.
type OrderItem struct {
	ProductName string          `json:"product_name"`
	Price       float64         `json:"price"`
	Quantity    int             `json:"quantity"`
	Attributes  []OrderItemAttr `json:"attributes"`
	LineTotal   float64         `json:"line_total"`
}

// OrderItemAttr is a chosen attribute option snapshot on an order item.
type OrderItemAttr struct {
	AttributeName string  `json:"attribute_name"`
	OptionValue   string  `json:"option_value"`
	PriceDelta    float64 `json:"price_delta"`
}

type CreateOrderGroupRequest struct {
	Label string `json:"label" validate:"omitempty,max=100"`
}

// CreatePublicOrderGroupRequest is the payload for the public tab-create
// endpoint. It embeds CreateOrderGroupRequest and adds a honeypot field: a
// legitimate client leaves Honeypot empty, so any value marks the request as a
// bot and it is silently dropped.
type CreatePublicOrderGroupRequest struct {
	CreateOrderGroupRequest
	Honeypot string `json:"website" validate:"max=0"`
}

type UpdateOrderGroupRequest struct {
	Label string `json:"label" validate:"omitempty,max=100"`
}

type UpdateOrderGroupStatusRequest struct {
	Status int8 `json:"status" validate:"required,oneof=1 2 3 4"`
}
