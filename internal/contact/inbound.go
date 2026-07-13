package contact

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// ResolveInboundInput defines deterministic contact resolution for inbound events.
type ResolveInboundInput struct {
	TeamID      uuid.UUID
	RawPhone    string
	ProfileName string
	Channel     string
	ExternalID  string
	Source      Source
}

// FindOrCreateFromInbound resolves or auto-creates a contact for Meta inbound events.
func (s *Service) FindOrCreateFromInbound(ctx context.Context, teamID uuid.UUID, rawPhone, profileName string) (*Contact, error) {
	return s.ResolveInbound(ctx, ResolveInboundInput{
		TeamID:      teamID,
		RawPhone:    rawPhone,
		ProfileName: profileName,
		Source:      SourceMetaInbound,
	})
}

// ResolveInbound resolves/creates contact with deterministic identity fallback.
func (s *Service) ResolveInbound(ctx context.Context, input ResolveInboundInput) (*Contact, error) {
	teamID := input.TeamID
	phoneNorm := normalizeInboundPhone(input.RawPhone)
	if phoneNorm == "" {
		return nil, errors.New("phone is required")
	}
	source := input.Source
	if source == "" {
		source = SourceMetaInbound
	}

	existing, err := s.contacts.FindByPhone(ctx, teamID, phoneNorm)
	if err == nil {
		return s.syncInboundNames(ctx, existing, input.ProfileName, phoneNorm)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	contactName, customerName := NewInboundNames(input.ProfileName, phoneNorm)

	now := time.Now().UTC()
	c := &Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		ContactName:     contactName,
		CustomerName:    customerName,
		Phone:           phoneNorm,
		Source:          source,
		MarketingStatus: MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.contacts.Create(ctx, c); err != nil {
		if existing, findErr := s.contacts.FindByPhone(ctx, teamID, phoneNorm); findErr == nil {
			return s.syncInboundNames(ctx, existing, input.ProfileName, phoneNorm)
		}
		return nil, err
	}
	c.PopulateLegacyName()
	return c, nil
}

func (s *Service) syncInboundNames(ctx context.Context, existing *Contact, profileName, phoneNorm string) (*Contact, error) {
	updated := false
	customerName := strings.TrimSpace(profileName)
	if customerName != "" && customerName != existing.CustomerName {
		existing.CustomerName = customerName
		updated = true
	}
	inboxName, _ := NewInboundNames(profileName, phoneNorm)
	if inboxName != "" && ShouldAutoUpdateContactName(existing) && inboxName != existing.ContactName {
		existing.ContactName = inboxName
		updated = true
	}
	if !updated {
		existing.PopulateLegacyName()
		return existing, nil
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.contacts.Update(ctx, existing); err != nil {
		return nil, err
	}
	existing.PopulateLegacyName()
	return existing, nil
}

func normalizeInboundPhone(raw string) string {
	digits := phone.Normalize(raw)
	if digits == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(raw), "+") {
		return "+" + digits
	}
	return "+" + digits
}
