package inbox

import (
	"time"

	"github.com/google/uuid"
)

const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

type Message struct {
	ID         uuid.UUID  `json:"id"`
	TeamID     uuid.UUID  `json:"team_id"`
	DeviceID   *uuid.UUID `json:"device_id,omitempty"`
	WAMID      *string    `json:"wamid,omitempty"`
	Sender     string     `json:"sender"`
	Recipient  *string    `json:"recipient,omitempty"`
	Content    string     `json:"content"`
	Direction  string     `json:"direction"`
	ReceivedAt time.Time  `json:"received_at"`
}

type Event struct {
	Type string  `json:"type"`
	Data Message `json:"data"`
}
