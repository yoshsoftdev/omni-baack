package recipient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
)

// ProviderDirectory is the narrow Canonical Provider surface used by the resolver.
// WhatsAppRecipientResolver is the only component that may query provider profiles
// for WhatsApp send addressing (Campaign + Inbox).
type ProviderDirectory interface {
	ListByContact(ctx context.Context, teamID, contactID uuid.UUID) ([]provider.Profile, error)
	ListByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]provider.Profile, error)
}

// WhatsAppRecipientResolver resolves WhatsApp send identity from Canonical Provider.
// Campaign and Inbox outbound must use this — never Contact.Phone.
type WhatsAppRecipientResolver struct {
	providers ProviderDirectory
}

// NewWhatsAppRecipientResolver constructs a resolver backed by Canonical Provider.
func NewWhatsAppRecipientResolver(providers ProviderDirectory) *WhatsAppRecipientResolver {
	return &WhatsAppRecipientResolver{providers: providers}
}

// Resolve returns the WhatsApp recipient for one contact.
func (r *WhatsAppRecipientResolver) Resolve(ctx context.Context, teamID, contactID uuid.UUID) (*WhatsAppRecipient, error) {
	if r == nil || r.providers == nil {
		return nil, fmt.Errorf("whatsapp recipient resolver: providers not configured")
	}
	profiles, err := r.providers.ListByContact(ctx, teamID, contactID)
	if err != nil {
		return nil, err
	}
	return pickWhatsAppRecipient(contactID, profiles)
}

// ResolveMany resolves WhatsApp recipients for many contacts in one provider lookup.
// Contacts without an active WhatsApp provider are omitted from the result map.
func (r *WhatsAppRecipientResolver) ResolveMany(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]*WhatsAppRecipient, error) {
	out := make(map[uuid.UUID]*WhatsAppRecipient, len(contactIDs))
	if len(contactIDs) == 0 {
		return out, nil
	}
	if r == nil || r.providers == nil {
		return nil, fmt.Errorf("whatsapp recipient resolver: providers not configured")
	}
	byContact, err := r.providers.ListByContactIDs(ctx, teamID, contactIDs)
	if err != nil {
		return nil, err
	}
	for _, contactID := range contactIDs {
		rec, err := pickWhatsAppRecipient(contactID, byContact[contactID])
		if err != nil {
			if errors.Is(err, ErrNoWhatsAppProvider) {
				continue
			}
			return nil, err
		}
		out[contactID] = rec
	}
	return out, nil
}

func pickWhatsAppRecipient(contactID uuid.UUID, profiles []provider.Profile) (*WhatsAppRecipient, error) {
	var candidates []provider.Profile
	for _, p := range profiles {
		if p.Provider != provider.WhatsApp {
			continue
		}
		if p.Status != provider.StatusActive {
			continue
		}
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		return nil, ErrNoWhatsAppProvider
	}

	chosen := candidates[0]
	for _, p := range candidates {
		if p.IsSourceOfTruth {
			chosen = p
			break
		}
	}

	phone := strings.TrimSpace(chosen.Phone)
	waID := strings.TrimSpace(chosen.ProviderUserID)
	if phone == "" {
		phone = waID
	}
	if !isValidWhatsAppAddress(phone) {
		return nil, ErrNoWhatsAppProvider
	}
	if waID == "" {
		waID = phone
	}

	return &WhatsAppRecipient{
		ContactID:         contactID,
		ProviderProfileID: chosen.ID,
		PhoneNumber:       phone,
		WAID:              waID,
	}, nil
}

func isValidWhatsAppAddress(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "ig:") {
		return false
	}
	digits := 0
	for _, r := range value {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return digits >= 8
}
