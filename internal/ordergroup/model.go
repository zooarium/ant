package ordergroup

import (
	"time"
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
}

// OrderSummary is a lightweight view of a member order within a group.
type OrderSummary struct {
	ID           int       `json:"id"`
	CustomerName string    `json:"customer_name"`
	Status       int8      `json:"status"`
	OrderedAt    time.Time `json:"ordered_at"`
	Total        float64   `json:"total"`
}

type CreateOrderGroupRequest struct {
	Label string `json:"label" validate:"omitempty,max=100"`
}

type UpdateOrderGroupRequest struct {
	Label string `json:"label" validate:"omitempty,max=100"`
}

type UpdateOrderGroupStatusRequest struct {
	Status int8 `json:"status" validate:"required,oneof=1 2 3 4"`
}
