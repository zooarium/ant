package attribute

import (
	"time"
)

type Attribute struct {
	ID        int       `json:"id"`
	AppID     int       `json:"app_id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	Status    int8      `json:"status"`
	Options   []Option  `json:"options"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Option struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

type CreateAttributeRequest struct {
	Name    string                `json:"name" validate:"required,max=100"`
	Status  *int8                 `json:"status" validate:"omitempty,oneof=0 1"`
	Options []CreateOptionRequest `json:"options" validate:"omitempty,dive"`
}

// UpdateAttributeRequest replaces the attribute and its options in one atomic
// call. Options are synced: id present = update value, id absent = create,
// existing option missing from the payload = delete.
type UpdateAttributeRequest struct {
	Name    string              `json:"name" validate:"required,max=100"`
	Status  *int8               `json:"status" validate:"required,oneof=0 1"`
	Options []SyncOptionRequest `json:"options" validate:"omitempty,dive"`
}

type CreateOptionRequest struct {
	Value string `json:"value" validate:"required,max=100"`
}

type SyncOptionRequest struct {
	ID    int    `json:"id" validate:"omitempty,min=1"`
	Value string `json:"value" validate:"required,max=100"`
}
