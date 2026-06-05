package device

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
)

type Device struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	Name            string
	PhoneNumber     *string
	WabaID          *string
	PhoneNumberID   *string
	AccessToken     *string
	Provider        string
	Status          Status
	LastConnectedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Response struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	PhoneNumber     *string    `json:"phone_number,omitempty"`
	WabaID          *string    `json:"waba_id,omitempty"`
	PhoneNumberID   *string    `json:"phone_number_id,omitempty"`
	Provider        string     `json:"provider"`
	Status          Status     `json:"status"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func ToResponse(d *Device) Response {
	return Response{
		ID:              d.ID,
		Name:            d.Name,
		PhoneNumber:     d.PhoneNumber,
		WabaID:          d.WabaID,
		PhoneNumberID:   d.PhoneNumberID,
		Provider:        d.Provider,
		Status:          d.Status,
		LastConnectedAt: d.LastConnectedAt,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	}
}
