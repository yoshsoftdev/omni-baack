package contact

import (
	"strings"

	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// ResolveInboxDisplayName returns the label shown in inbox UI.
func ResolveInboxDisplayName(c *Contact) string {
	if c == nil {
		return ""
	}
	if name := strings.TrimSpace(c.ContactName); name != "" {
		return name
	}
	if name := strings.TrimSpace(c.CustomerName); name != "" {
		return name
	}
	return strings.TrimSpace(c.Phone)
}

// NewInboundNames derives inbox and customer names from an inbound WhatsApp profile.
func NewInboundNames(profileName, phone string) (contactName, customerName string) {
	customerName = strings.TrimSpace(profileName)
	contactName = customerName
	if contactName == "" {
		contactName = strings.TrimSpace(phone)
	}
	return contactName, customerName
}

// ShouldAutoUpdateContactName reports whether Meta sync may overwrite the inbox display name.
func ShouldAutoUpdateContactName(c *Contact) bool {
	if c == nil {
		return true
	}
	contactName := strings.TrimSpace(c.ContactName)
	if contactName == "" {
		return true
	}
	if isPhoneOnlyContactName(contactName, c.Phone) {
		return true
	}
	if contactName == strings.TrimSpace(c.CustomerName) {
		return true
	}
	return false
}

func isPhoneOnlyContactName(name, phone string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	phoneDigits := normalizeDigits(phone)
	nameDigits := normalizeDigits(trimmed)
	if phoneDigits != "" && nameDigits == phoneDigits {
		return true
	}
	return trimmed == strings.TrimSpace(phone)
}

func normalizeDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// PopulateLegacyName sets the deprecated API alias before serialization.
func (c *Contact) PopulateLegacyName() {
	if c == nil {
		return
	}
	c.Name = c.ContactName
}

func resolveCreateNames(input CreateInput) (contactName, customerName string, err error) {
	contactName = strings.TrimSpace(input.ContactName)
	if contactName == "" {
		contactName = strings.TrimSpace(input.Name)
	}
	if contactName == "" {
		return "", "", serviceerrors.NewValidation("contact_name", "is required")
	}
	customerName = contactName
	if input.CustomerName != nil {
		customerName = strings.TrimSpace(*input.CustomerName)
		if customerName == "" {
			return "", "", serviceerrors.NewValidation("customer_name", "is required")
		}
	}
	return contactName, customerName, nil
}

func resolveUpdateContactName(input UpdateInput) (string, error) {
	if input.ContactName != nil {
		contactName := strings.TrimSpace(*input.ContactName)
		if contactName == "" {
			return "", serviceerrors.NewValidation("contact_name", "is required")
		}
		return contactName, nil
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return "", serviceerrors.NewValidation("name", "is required")
		}
		return name, nil
	}
	return "", serviceerrors.NewValidation("contact_name", "is required")
}
