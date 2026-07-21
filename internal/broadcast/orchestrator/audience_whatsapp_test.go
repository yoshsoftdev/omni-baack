package orchestrator

import (
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/contact"
)

func TestIsWhatsAppEligibleContact(t *testing.T) {
	if !isWhatsAppEligibleContact(contact.Contact{Phone: "+6281234567890", Source: contact.SourceWhatsApp}) {
		t.Fatal("expected whatsapp phone to be eligible")
	}
	if isWhatsAppEligibleContact(contact.Contact{Phone: "ig:1028733076273916", Source: contact.SourceInstagram}) {
		t.Fatal("expected instagram contact to be ineligible")
	}
	if isWhatsAppEligibleContact(contact.Contact{Phone: "ig:1028733076273916", Source: contact.SourceManual}) {
		t.Fatal("expected ig: phone to be ineligible")
	}
	if isWhatsAppEligibleContact(contact.Contact{Phone: "123", Source: contact.SourceManual}) {
		t.Fatal("expected short phone to be ineligible")
	}
}
