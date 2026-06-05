package product

import (
	"time"
)

type Product struct {
	ID         int                 `json:"id"`
	AppID      int                 `json:"app_id"`
	UserID     int                 `json:"user_id"`
	Name       string              `json:"name"`
	Price      float64             `json:"price"`
	Status     int8                `json:"status"`
	Attributes []AssignedAttribute `json:"attributes,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// AssignedAttribute is an attribute glued to a product, with the options a
// caller can choose from when adding the product to an order.
type AssignedAttribute struct {
	AttributeID int               `json:"attribute_id"`
	Name        string            `json:"name"`
	IsMandatory bool              `json:"is_mandatory"`
	Options     []AttributeOption `json:"options"`
}

type AttributeOption struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

type AttributeAssignmentRequest struct {
	AttributeID int  `json:"attribute_id" validate:"required"`
	IsMandatory bool `json:"is_mandatory"`
}

type CreateProductRequest struct {
	Name       string                       `json:"name" validate:"required,max=200"`
	Price      float64                      `json:"price" validate:"gte=0"`
	Status     *int8                        `json:"status" validate:"omitempty,oneof=0 1"`
	Attributes []AttributeAssignmentRequest `json:"attributes" validate:"omitempty,dive"`
}

// UpdateProductRequest replaces the product's attribute assignments with the
// given set (full sync: add new, update flags, remove missing).
type UpdateProductRequest struct {
	Name       string                       `json:"name" validate:"required,max=200"`
	Price      float64                      `json:"price" validate:"gte=0"`
	Status     *int8                        `json:"status" validate:"required,oneof=0 1"`
	Attributes []AttributeAssignmentRequest `json:"attributes" validate:"omitempty,dive"`
}
