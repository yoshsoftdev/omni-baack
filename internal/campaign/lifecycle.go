package campaign

// CanTransitionCampaignStatus reports whether a campaign status change is allowed.
func CanTransitionCampaignStatus(from, to Status) bool {
	if from == to {
		return false
	}
	switch from {
	case StatusDraft:
		return to == StatusScheduled || to == StatusQueued || to == StatusCancelled
	case StatusScheduled:
		return to == StatusQueued || to == StatusCancelled
	case StatusQueued:
		return to == StatusSending || to == StatusCancelled
	case StatusSending:
		return to == StatusCompleted || to == StatusFailed || to == StatusCancelled
	case StatusCompleted, StatusFailed, StatusCancelled:
		return false
	default:
		return false
	}
}

// CanSendCampaign reports whether POST /campaigns/:id/send is valid for the current status.
func CanSendCampaign(status Status) bool {
	return status == StatusDraft || status == StatusScheduled
}

// CanCancelCampaign reports whether POST /campaigns/:id/cancel is valid for the current status.
func CanCancelCampaign(status Status) bool {
	switch status {
	case StatusDraft, StatusScheduled, StatusQueued, StatusSending:
		return true
	default:
		return false
	}
}

// CanCancelRecipient reports whether a recipient may be cancelled (Phase 8 rules).
func CanCancelRecipient(status RecipientStatus, hasWAMID bool) bool {
	switch status {
	case RecipientPending, RecipientQueued:
		return true
	case RecipientSending:
		return !hasWAMID
	default:
		return false
	}
}
