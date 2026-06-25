package inbox

import "github.com/omnichannel/omni-chan-v2/internal/conversation"

// NextStatusOnInbound returns the conversation status after an inbound message.
func NextStatusOnInbound(current conversation.Status) conversation.Status {
	switch current {
	case conversation.StatusResolved, conversation.StatusClosed:
		return conversation.StatusOpen
	default:
		return current
	}
}

// ShouldReopen reports whether an inbound message reopens the conversation.
func ShouldReopen(current conversation.Status) bool {
	return current == conversation.StatusResolved || current == conversation.StatusClosed
}
