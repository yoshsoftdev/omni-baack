package contact

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
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

	if s.providers != nil {
		profile, profileErr := s.providers.FindByExternalID(ctx, teamID, provider.WhatsApp, phoneNorm)
		if profileErr == nil {
			now := time.Now().UTC()
			profile.DisplayName = strings.TrimSpace(input.ProfileName)
			profile.Phone = phoneNorm
			profile.LastObservedAt = &now
			profile.Status = provider.StatusActive
			if err := s.providers.UpdateObserved(ctx, profile); err != nil {
				return nil, err
			}
			existing, err := s.contacts.FindByID(ctx, teamID, profile.ContactID)
			if err != nil {
				return nil, err
			}
			if profile.IsSourceOfTruth && existing.Phone != phoneNorm {
				existing.Phone = phoneNorm
				existing.UpdatedAt = now
				if err := s.contacts.Update(ctx, existing); err != nil {
					return nil, err
				}
			}
			return s.syncInboundNames(ctx, existing, input.ProfileName, phoneNorm)
		}
		if !errors.Is(profileErr, repository.ErrNotFound) {
			return nil, profileErr
		}
	}

	existing, err := s.contacts.FindByPhone(ctx, teamID, phoneNorm)
	if err == nil {
		if err := s.observeWhatsAppProfile(ctx, existing, input.ProfileName, phoneNorm); err != nil {
			return nil, err
		}
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
			if observeErr := s.observeWhatsAppProfile(ctx, existing, input.ProfileName, phoneNorm); observeErr != nil {
				return nil, observeErr
			}
			return s.syncInboundNames(ctx, existing, input.ProfileName, phoneNorm)
		}
		return nil, err
	}
	if err := s.observeWhatsAppProfile(ctx, c, input.ProfileName, phoneNorm); err != nil {
		return nil, err
	}
	c.PopulateLegacyName()
	return c, nil
}

func (s *Service) observeWhatsAppProfile(ctx context.Context, contactEntity *Contact, profileName, phone string) error {
	if s.providers == nil || contactEntity == nil {
		return nil
	}
	now := time.Now().UTC()
	profile := &provider.Profile{
		ID:             uuid.New(),
		TeamID:         contactEntity.TeamID,
		ContactID:      contactEntity.ID,
		Provider:       provider.WhatsApp,
		ProviderUserID: phone,
		DisplayName:    strings.TrimSpace(profileName),
		Phone:          phone,
		ConnectedAt:    now,
		LastObservedAt: &now,
		Status:         provider.StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.providers.Create(ctx, profile); err != nil {
		if existing, findErr := s.providers.FindByExternalID(ctx, contactEntity.TeamID, provider.WhatsApp, phone); findErr == nil {
			return s.providers.UpdateObserved(ctx, existing)
		}
		return err
	}
	contactEntity.Providers = append(contactEntity.Providers, *profile)
	return nil
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
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return nil, errors.New("instagram sender id is required")
	}
	if s.providers != nil {
		return s.resolveCanonicalInstagramInbound(ctx, teamID, senderID, displayName, username, avatarURL)
	}

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

func (s *Service) resolveCanonicalInstagramInbound(ctx context.Context, teamID uuid.UUID, senderID, displayName, username, avatarURL string) (*Contact, error) {
	now := time.Now().UTC()
	profile, err := s.providers.FindByExternalID(ctx, teamID, provider.Instagram, senderID)
	if err == nil {
		profile.DisplayName = strings.TrimSpace(displayName)
		profile.Username = strings.TrimPrefix(strings.TrimSpace(username), "@")
		profile.AvatarURL = strings.TrimSpace(avatarURL)
		profile.LastObservedAt = &now
		profile.Status = provider.StatusActive
		if err := s.providers.UpdateObserved(ctx, profile); err != nil {
			return nil, err
		}
		return s.contacts.FindByID(ctx, teamID, profile.ContactID)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	profileName := strings.TrimSpace(displayName)
	if profileName == "" {
		profileName = strings.TrimSpace(username)
	}
	if profileName == "" {
		profileName = "Instagram User"
	}
	contactEntity := &Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		ContactName:     profileName,
		CustomerName:    profileName,
		Source:          SourceManual,
		MarketingStatus: MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.contacts.Create(ctx, contactEntity); err != nil {
		return nil, err
	}
	profile = &provider.Profile{
		ID:             uuid.New(),
		TeamID:         teamID,
		ContactID:      contactEntity.ID,
		Provider:       provider.Instagram,
		ProviderUserID: senderID,
		DisplayName:    strings.TrimSpace(displayName),
		Username:       strings.TrimPrefix(strings.TrimSpace(username), "@"),
		AvatarURL:      strings.TrimSpace(avatarURL),
		ConnectedAt:    now,
		LastObservedAt: &now,
		Status:         provider.StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.providers.Create(ctx, profile); err != nil {
		if existing, findErr := s.providers.FindByExternalID(ctx, teamID, provider.Instagram, senderID); findErr == nil {
			_ = s.contacts.SoftDelete(ctx, teamID, contactEntity.ID)
			return s.contacts.FindByID(ctx, teamID, existing.ContactID)
		}
		return nil, err
	}
	contactEntity.Providers = []provider.Profile{*profile}
	contactEntity.PopulateLegacyName()
	return contactEntity, nil
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
