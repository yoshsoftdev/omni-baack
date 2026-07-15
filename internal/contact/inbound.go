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

// ResolveInstagramInbound resolves/creates a contact for Instagram DM webhooks.
// displayName, username, and avatarURL come from the Instagram User Profile API when available.
func (s *Service) ResolveInstagramInbound(ctx context.Context, teamID uuid.UUID, senderID, displayName, username, avatarURL string) (*Contact, error) {
	phoneKey := instagramContactKey(senderID)
	if phoneKey == "ig:" {
		return nil, errors.New("instagram sender id is required")
	}

	profileName := strings.TrimSpace(displayName)
	if profileName == "" {
		profileName = strings.TrimSpace(username)
	}
	if strings.TrimSpace(username) != "" && !strings.HasPrefix(profileName, "@") {
		if profileName == "" {
			profileName = "@" + strings.TrimPrefix(strings.TrimSpace(username), "@")
		}
	}
	avatarURL = strings.TrimSpace(avatarURL)

	existing, err := s.contacts.FindByPhone(ctx, teamID, phoneKey)
	if err == nil {
		return s.syncInstagramInboundProfile(ctx, existing, profileName, username, avatarURL)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	contactName := strings.TrimSpace(profileName)
	if contactName == "" {
		contactName = "Instagram User"
	}
	customerName := contactName

	now := time.Now().UTC()
	c := &Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		ContactName:     contactName,
		CustomerName:    customerName,
		Phone:           phoneKey,
		Source:          SourceInstagram,
		MarketingStatus: MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if avatarURL != "" {
		c.AvatarURL = &avatarURL
	}
	if err := s.contacts.Create(ctx, c); err != nil {
		if existing, findErr := s.contacts.FindByPhone(ctx, teamID, phoneKey); findErr == nil {
			return s.syncInstagramInboundProfile(ctx, existing, profileName, username, avatarURL)
		}
		return nil, err
	}
	c.PopulateLegacyName()
	return c, nil
}

func (s *Service) syncInstagramInboundProfile(ctx context.Context, existing *Contact, profileName, username, avatarURL string) (*Contact, error) {
	updated := false
	name := strings.TrimSpace(profileName)
	if name == "" {
		name = strings.TrimSpace(username)
		if name != "" && !strings.HasPrefix(name, "@") {
			name = "@" + strings.TrimPrefix(name, "@")
		}
	}
	if name != "" && name != existing.ContactName && ShouldAutoUpdateContactName(existing) {
		existing.ContactName = name
		updated = true
	}
	if name != "" && name != existing.CustomerName {
		existing.CustomerName = name
		updated = true
	}
	if existing.Source != SourceInstagram {
		existing.Source = SourceInstagram
		updated = true
	}
	if avatarURL != "" {
		if existing.AvatarURL == nil || strings.TrimSpace(*existing.AvatarURL) != avatarURL {
			existing.AvatarURL = &avatarURL
			updated = true
		}
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

func instagramContactKey(senderID string) string {
	return "ig:" + strings.TrimSpace(senderID)
}

// ParseInstagramScopedID extracts the Instagram scoped user id from a contact phone key (ig:{id}).
func ParseInstagramScopedID(phone string) (string, bool) {
	phone = strings.TrimSpace(phone)
	if !strings.HasPrefix(phone, "ig:") {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(phone, "ig:"))
	if id == "" {
		return "", false
	}
	return id, true
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
