package recipient

import (
	"github.com/google/uuid"
)

// WhatsAppRecipient is the resolved WhatsApp send identity for a contact.
// Campaign engine code must use this type — never Contact.Phone / provider rows directly.
type WhatsAppRecipient struct {
	ContactID         uuid.UUID
	ProviderProfileID uuid.UUID
	PhoneNumber       string
	WAID              string
	// PhoneNumberID is the business Cloud API phone_number_id when known on the profile channel.
	// Usually supplied by the campaign Channel at send time; optional here.
	PhoneNumberID *string
}
