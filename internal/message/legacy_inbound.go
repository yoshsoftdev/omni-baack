package message

import (
	"context"

	"github.com/google/uuid"
)

// LegacyInboundMessage is the webhook payload accepted by the legacy inbound stub.
// Kept in the message domain so message need not import pkg/meta.
type LegacyInboundMessage struct {
	PhoneNumberID string
	From          string
	WAMID         string
	Content       string
	ContactID     uuid.UUID
}

// HandleMetaInbound is a compatibility stub for the NATS webhook consumer.
// Full inbound routing lives in the event consumer / inbox sync path.
func (s *Service) HandleMetaInbound(ctx context.Context, _ LegacyInboundMessage) error {
	_ = ctx
	return nil
}
