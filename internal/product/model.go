package product

import (
	"time"
)

type Product struct {
	ID        int       `json:"id"`
	AppID     int       `json:"app_id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateProductRequest struct {
	Name   string `json:"name" validate:"required"`
	Status int8   `json:"status" validate:"omitempty,oneof=0 1"`
}

type UpdateProductRequest struct {
	Name   string `json:"name" validate:"required"`
	Status int8   `json:"status" validate:"omitempty,oneof=0 1"`
}
