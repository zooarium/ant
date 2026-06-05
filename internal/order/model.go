package order

import (
	"time"
)

type Order struct {
	ID        int       `json:"id"`
	AppID     int       `json:"app_id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateOrderRequest struct {
	Name   string `json:"name" validate:"required"`
	Status int8   `json:"status" validate:"omitempty,oneof=0 1"`
}

type UpdateOrderRequest struct {
	Name   string `json:"name" validate:"required"`
	Status int8   `json:"status" validate:"omitempty,oneof=0 1"`
}
