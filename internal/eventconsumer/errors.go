package eventconsumer

import (
	"context"

	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
	"github.com/omnichannel/omni-chan-v2/internal/consumererrors"
)

// Re-export consumer error helpers for handlers in this package.
var (
	ErrUnknownChannel = consumererrors.ErrUnknownChannel
	ErrMalformedEvent = consumererrors.ErrMalformedEvent
)

func TerminalDLQ(category string, err error) error {
	return consumererrors.TerminalDLQ(category, err)
}

func IsTerminal(err error) (*consumererrors.TerminalError, bool) {
	return consumererrors.IsTerminal(err)
}

// ConsumerDLQ publishes failed consumer events.
type ConsumerDLQ interface {
	PublishConsumerDLQ(ctx context.Context, category string, errMsg string, event sharedevents.CanonicalEvent) error
}
