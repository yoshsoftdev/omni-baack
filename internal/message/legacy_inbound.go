package message

import (
	"context"

	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// HandleMetaInbound is a compatibility stub for the NATS webhook consumer.
// Full inbound routing will be restored when inbox services are rewired.
func (s *Service) HandleMetaInbound(ctx context.Context, _ metaclient.InboundMessage) error {
	_ = ctx
	return nil
}
