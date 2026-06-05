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
)

type Order struct {
	ID              int         `json:"id"`
	AppID           int         `json:"app_id"`
	UserID          int         `json:"user_id"`
	DivisionID      int         `json:"division_id"`
	CustomerName    string      `json:"customer_name"`
	CustomerContact string      `json:"customer_contact"`
	Status          int8        `json:"status"`
	ProductsCount   int         `json:"products_count"`
	Products        []OrderItem `json:"products,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
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
}

type OrderItemAttribute struct {
	AttributeID   int    `json:"attribute_id"`
	AttributeName string `json:"attribute_name"`
	OptionID      int    `json:"option_id"`
	OptionValue   string `json:"option_value"`
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
	CustomerName    string             `json:"customer_name" validate:"required,max=100"`
	CustomerContact string             `json:"customer_contact" validate:"required,min=7,max=20"`
	Status          *int8              `json:"status" validate:"omitempty,oneof=1 2 3 4"`
	Products        []OrderItemRequest `json:"products" validate:"required,min=1,dive"`
}

// UpdateOrderRequest atomically replaces the order's customer details and
// syncs its items in one call. Items are synced by id: an item with an id
// keeps its product/attribute snapshot and only its quantity is editable; an
// item without an id is added (product_id + attributes required); existing
// items missing from the payload are deleted. Status is managed separately
// via /orders/{id}/status.
type UpdateOrderRequest struct {
	CustomerName    string                 `json:"customer_name" validate:"required,max=100"`
	CustomerContact string                 `json:"customer_contact" validate:"required,min=7,max=20"`
	Products        []SyncOrderItemRequest `json:"products" validate:"required,min=1,dive"`
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
	Status int8 `json:"status" validate:"required,oneof=1 2 3 4"`
}
