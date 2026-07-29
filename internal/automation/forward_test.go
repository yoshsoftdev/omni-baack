package automation

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

func TestWebhookSubscribes(t *testing.T) {
	if !webhookSubscribes(nil, EventMessageReceived) {
		t.Fatal("empty events should default to message.received")
	}
	if !webhookSubscribes([]string{EventMessageReceived}, EventMessageReceived) {
		t.Fatal("expected subscribed")
	}
	if webhookSubscribes([]string{"other.event"}, EventMessageReceived) {
		t.Fatal("expected not subscribed")
	}
}

func TestNormalizeForwardEvents(t *testing.T) {
	got := normalizeForwardEvents([]string{" message.received ", "unknown", "message.received"})
	if len(got) != 1 || got[0] != EventMessageReceived {
		t.Fatalf("unexpected %#v", got)
	}
	got = normalizeForwardEvents(nil)
	if len(got) != 1 || got[0] != EventMessageReceived {
		t.Fatalf("unexpected default %#v", got)
	}
}

func TestEligibleForUnassignedForward(t *testing.T) {
	if !eligibleForUnassignedForward(ForwardInboundInput{
		ConversationStatus: conversation.StatusOpen,
		AssignedTo:         nil,
	}) {
		t.Fatal("open + unassigned should be eligible")
	}

	assignee := uuid.New()
	if eligibleForUnassignedForward(ForwardInboundInput{
		ConversationStatus: conversation.StatusOpen,
		AssignedTo:         &assignee,
	}) {
		t.Fatal("assigned conversation should not be eligible")
	}

	if eligibleForUnassignedForward(ForwardInboundInput{
		ConversationStatus: conversation.StatusPending,
		AssignedTo:         nil,
	}) {
		t.Fatal("non-open conversation should not be eligible")
	}
}

func TestForwardAssignedConversationNeverEligible(t *testing.T) {
	assignee := uuid.New()
	input := ForwardInboundInput{
		ConversationStatus: conversation.StatusOpen,
		AssignedTo:         &assignee,
	}
	if eligibleForUnassignedForward(input) {
		t.Fatal("assigned conversation should never be eligible")
	}
}
