package conversation

import (
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
)

// Item is the inbox-facing conversation projection with contact labels.
type Item struct {
	Conversation
	ContactName  string `json:"contact_name"`
	CustomerName string `json:"customer_name,omitempty"`
	ContactPhone string `json:"contact_phone"`
	DeviceID     string `json:"device_id,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
}

func toItem(conv Conversation, contactMap map[uuid.UUID]contact.Contact) Item {
	item := Item{Conversation: conv}
	if conv.ChannelID != nil {
		item.DeviceID = conv.ChannelID.String()
	}
	ct, ok := contactMap[conv.ContactID]
	if !ok {
		return item
	}
	item.ContactName = contact.ResolveInboxDisplayName(&ct)
	item.CustomerName = ct.CustomerName
	item.ContactPhone = ct.Phone
	return item
}

func toItems(convs []Conversation, contactMap map[uuid.UUID]contact.Contact) []Item {
	items := make([]Item, len(convs))
	for i := range convs {
		items[i] = toItem(convs[i], contactMap)
	}
	return items
}

// ToRealtimeItem builds the websocket/API payload for a conversation update.
func ToRealtimeItem(conv *Conversation, ct *contact.Contact) Item {
	if conv == nil {
		return Item{}
	}
	contactMap := map[uuid.UUID]contact.Contact{}
	if ct != nil {
		contactMap[conv.ContactID] = *ct
	}
	return toItem(*conv, contactMap)
}
