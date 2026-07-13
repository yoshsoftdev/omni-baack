package conversation

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
)

func TestToRealtimeItemUsesContactDisplayName(t *testing.T) {
	teamID := uuid.New()
	contactID := uuid.New()
	convID := uuid.New()

	conv := &Conversation{
		ID:        convID,
		TeamID:    teamID,
		ContactID: contactID,
		Status:    StatusOpen,
	}
	ct := &contact.Contact{
		ID:           contactID,
		TeamID:       teamID,
		ContactName:  "VIP Client",
		CustomerName: "YoShandy",
		Phone:        "+6282288812003",
	}

	item := ToRealtimeItem(conv, ct)
	if item.ContactName != "VIP Client" {
		t.Fatalf("contact_name = %q", item.ContactName)
	}
	if item.CustomerName != "YoShandy" {
		t.Fatalf("customer_name = %q", item.CustomerName)
	}
	if item.ContactPhone != "+6282288812003" {
		t.Fatalf("contact_phone = %q", item.ContactPhone)
	}
}
