package ordergroup

import (
	"time"

	"ant/internal/order"
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
	// Total is the sum of all member orders' pre-tax totals. Populated on
	// detail reads.
	Total float64 `json:"total"`
	// TaxTotal is the sum over member orders of total * tax_percent / 100,
	// computed per order so mixed rates within a group (rate changed mid-tab)
	// stay correct. Populated on detail reads.
	TaxTotal float64 `json:"tax_total"`
	// GrandTotal is Total + TaxTotal. Populated on detail reads.
	GrandTotal float64 `json:"grand_total"`
	// Orders is the member order summary list. Populated on detail reads.
	Orders    []OrderSummary `json:"orders,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// OrderSummary is a lightweight view of a member order within a group.
type OrderSummary struct {
	ID           int       `json:"id"`
	CustomerName string    `json:"customer_name"`
	Status       int8      `json:"status"`
	OrderedAt    time.Time `json:"ordered_at"`
	// TaxPercent is the tax rate applied to this order as a percentage.
	TaxPercent float64 `json:"tax_percent"`
	// Total is the pre-tax order amount.
	Total float64 `json:"total"`
	// Products are the order's snapshotted items (already eager-loaded for the
	// total computation), so a public group view can print the full tab.
	Products []order.OrderItem `json:"products"`
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
