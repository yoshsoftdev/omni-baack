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

// FindOrCreateFromInbound resolves or auto-creates a contact for Meta inbound events.
func (s *Service) FindOrCreateFromInbound(ctx context.Context, teamID uuid.UUID, rawPhone, name string) (*Contact, error) {
	phoneNorm := normalizeInboundPhone(rawPhone)
	if phoneNorm == "" {
		return nil, errors.New("phone is required")
	}

	existing, err := s.contacts.FindByPhone(ctx, teamID, phoneNorm)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = phoneNorm
	}

	now := time.Now().UTC()
	c := &Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		Name:            displayName,
		Phone:           phoneNorm,
		Source:          SourceMetaInbound,
		MarketingStatus: MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.contacts.Create(ctx, c); err != nil {
		if existing, findErr := s.contacts.FindByPhone(ctx, teamID, phoneNorm); findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return c, nil
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
