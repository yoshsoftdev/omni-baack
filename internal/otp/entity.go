package otp

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusSent   Status = "sent"
	StatusFailed Status = "failed"
)

type Log struct {
	ID           uuid.UUID
	TeamID       uuid.UUID
	DeviceID     uuid.UUID
	Phone        string
	Wamid        *string
	Status       Status
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type SendResponse struct {
	ID     uuid.UUID `json:"id"`
	Phone  string    `json:"phone"`
	Wamid  string    `json:"wamid"`
	Status Status    `json:"status"`
}
