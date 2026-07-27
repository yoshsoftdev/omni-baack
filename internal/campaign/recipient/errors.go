package recipient

import "errors"

var (
	// ErrNoWhatsAppProvider is returned when a contact has no active WhatsApp provider profile.
	ErrNoWhatsAppProvider = errors.New("contact does not have a WhatsApp provider")

	// ErrNoEligibleRecipients is returned when audience resolution yields zero WhatsApp-capable recipients.
	ErrNoEligibleRecipients = errors.New("no eligible WhatsApp recipients")
)
