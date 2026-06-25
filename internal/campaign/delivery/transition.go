package delivery

import (
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// RecipientStatusFromEvent maps canonical Meta status events to recipient status.
func RecipientStatusFromEvent(eventType string) campaign.RecipientStatus {
	switch eventType {
	case sharedevents.TypeMessageStatusSent:
		return campaign.RecipientSent
	case sharedevents.TypeMessageStatusDelivered:
		return campaign.RecipientDelivered
	case sharedevents.TypeMessageStatusRead:
		return campaign.RecipientRead
	case sharedevents.TypeMessageStatusFailed:
		return campaign.RecipientFailed
	default:
		return ""
	}
}

// CanTransition reports whether a recipient status change is allowed (mirrors DB trigger).
func CanTransition(from, to campaign.RecipientStatus) bool {
	if from == to {
		return false
	}
	switch from {
	case campaign.RecipientRead, campaign.RecipientFailed, campaign.RecipientCancelled, campaign.RecipientSkipped:
		return false
	case campaign.RecipientPending:
		return to == campaign.RecipientQueued || to == campaign.RecipientSkipped ||
			to == campaign.RecipientCancelled || to == campaign.RecipientFailed
	case campaign.RecipientQueued:
		return to == campaign.RecipientSending || to == campaign.RecipientCancelled || to == campaign.RecipientFailed
	case campaign.RecipientSending:
		return to == campaign.RecipientSent || to == campaign.RecipientQueued ||
			to == campaign.RecipientFailed || to == campaign.RecipientCancelled
	case campaign.RecipientSent:
		return to == campaign.RecipientDelivered || to == campaign.RecipientRead || to == campaign.RecipientFailed
	case campaign.RecipientDelivered:
		return to == campaign.RecipientRead
	default:
		return false
	}
}

// CounterDeltaForTransition returns campaign counter increments for a successful transition.
func CounterDeltaForTransition(from, to campaign.RecipientStatus) campaign.CounterDelta {
	var d campaign.CounterDelta
	if to == campaign.RecipientSent && from != campaign.RecipientSent {
		d.Sent = 1
	}
	if to == campaign.RecipientDelivered && from != campaign.RecipientDelivered && from != campaign.RecipientRead {
		d.Delivered = 1
	}
	if to == campaign.RecipientRead && from != campaign.RecipientRead {
		d.Read = 1
	}
	if to == campaign.RecipientFailed && from != campaign.RecipientFailed {
		d.Failed = 1
	}
	return d
}

func isTerminal(status campaign.RecipientStatus) bool {
	switch status {
	case campaign.RecipientRead, campaign.RecipientFailed, campaign.RecipientCancelled, campaign.RecipientSkipped:
		return true
	default:
		return false
	}
}
