package inbox

import (
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

// PriorityEvent triggers priority recalculation on business events only.
type PriorityEvent string

const (
	PriorityEventInbound    PriorityEvent = "inbound"
	PriorityEventOutbound   PriorityEvent = "outbound"
	PriorityEventAssignment PriorityEvent = "assignment"
	PriorityEventClose      PriorityEvent = "close"
	PriorityEventReopen     PriorityEvent = "reopen"
	PriorityEventMarkRead   PriorityEvent = "mark_read"
)

// PriorityUpdate holds recalculated inbox priority fields.
type PriorityUpdate struct {
	WaitingSince  *time.Time
	BasePriority  int
	PriorityScore int64
}

// ComputeBasePriority derives static urgency from conversation state.
func ComputeBasePriority(conv *conversation.Conversation) int {
	if conv == nil {
		return 0
	}
	if conv.Status == conversation.StatusClosed || conv.Status == conversation.StatusResolved {
		return -200
	}
	base := 0
	unreadBonus := conv.UnreadCount * 10
	if unreadBonus > 50 {
		unreadBonus = 50
	}
	base += unreadBonus
	if conv.AssignedTo == nil && (conv.Status == conversation.StatusOpen || conv.Status == conversation.StatusPending) {
		base += 100
	}
	return base
}

// AgeBonus returns the aging component from waiting time at event time.
func AgeBonus(waitingSince *time.Time, now time.Time) int {
	if waitingSince == nil || waitingSince.IsZero() {
		return 0
	}
	mins := int(now.Sub(*waitingSince).Minutes())
	if mins < 0 {
		return 0
	}
	switch {
	case mins < 1:
		return 0
	case mins < 2:
		return 10
	case mins < 5:
		return 20
	case mins < 15:
		return 50
	case mins < 16:
		return 100
	default:
		return 150
	}
}

// RecalculatePriority computes waiting_since, base_priority, and priority_score.
func RecalculatePriority(conv *conversation.Conversation, event PriorityEvent, now time.Time) PriorityUpdate {
	if conv == nil {
		return PriorityUpdate{}
	}

	waitingSince := conv.WaitingSince
	switch event {
	case PriorityEventInbound:
		waitingSince = &now
	case PriorityEventOutbound, PriorityEventClose:
		waitingSince = nil
	case PriorityEventReopen:
		if customerWaiting(conv) {
			t := now
			if conv.LastCustomerMessageAt != nil {
				t = *conv.LastCustomerMessageAt
			}
			waitingSince = &t
		}
	case PriorityEventAssignment, PriorityEventMarkRead:
		// waiting_since unchanged; base_priority recalculated from current state.
	}

	// Apply post-event state for base priority calculation.
	working := *conv
	working.WaitingSince = waitingSince
	if event == PriorityEventMarkRead {
		working.UnreadCount = 0
	}
	if event == PriorityEventClose {
		working.Status = conversation.StatusClosed
	}

	base := ComputeBasePriority(&working)
	score := int64(base + AgeBonus(waitingSince, now))

	return PriorityUpdate{
		WaitingSince:  waitingSince,
		BasePriority:  base,
		PriorityScore: score,
	}
}

func customerWaiting(conv *conversation.Conversation) bool {
	if conv.LastCustomerMessageAt == nil {
		return false
	}
	if conv.LastAgentMessageAt == nil {
		return true
	}
	return conv.LastCustomerMessageAt.After(*conv.LastAgentMessageAt)
}
