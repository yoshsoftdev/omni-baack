package contact

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type igContactRepoStub struct {
	Repository
	contacts map[uuid.UUID]*Contact
}

func (s *igContactRepoStub) FindByID(_ context.Context, teamID, id uuid.UUID) (*Contact, error) {
	item, ok := s.contacts[id]
	if !ok || item.TeamID != teamID || item.DeletedAt != nil {
		return nil, repository.ErrNotFound
	}
	copy := *item
	if item.AvatarURL != nil {
		avatar := *item.AvatarURL
		copy.AvatarURL = &avatar
	}
	return &copy, nil
}

func (s *igContactRepoStub) Create(_ context.Context, c *Contact) error {
	copy := *c
	if c.AvatarURL != nil {
		avatar := *c.AvatarURL
		copy.AvatarURL = &avatar
	}
	s.contacts[c.ID] = &copy
	return nil
}

func (s *igContactRepoStub) Update(_ context.Context, c *Contact) error {
	if _, ok := s.contacts[c.ID]; !ok {
		return repository.ErrNotFound
	}
	copy := *c
	if c.AvatarURL != nil {
		avatar := *c.AvatarURL
		copy.AvatarURL = &avatar
	}
	s.contacts[c.ID] = &copy
	return nil
}

func (s *igContactRepoStub) SoftDelete(_ context.Context, teamID, id uuid.UUID) error {
	item, ok := s.contacts[id]
	if !ok || item.TeamID != teamID {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	item.DeletedAt = &now
	return nil
}

type igProviderRepoStub struct {
	provider.Repository
	byExternal map[string]*provider.Profile
	byID       map[uuid.UUID]*provider.Profile
}

func (s *igProviderRepoStub) FindByExternalID(
	_ context.Context,
	teamID uuid.UUID,
	kind provider.Kind,
	providerUserID string,
) (*provider.Profile, error) {
	item, ok := s.byExternal[providerUserID]
	if !ok || item.TeamID != teamID || item.Provider != kind {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (s *igProviderRepoStub) UpdateObserved(_ context.Context, profile *provider.Profile) error {
	item, ok := s.byID[profile.ID]
	if !ok {
		return repository.ErrNotFound
	}
	item.DisplayName = profile.DisplayName
	item.Username = profile.Username
	item.AvatarURL = profile.AvatarURL
	item.LastObservedAt = profile.LastObservedAt
	item.Status = profile.Status
	return nil
}

func (s *igProviderRepoStub) Create(_ context.Context, profile *provider.Profile) error {
	copy := *profile
	s.byID[profile.ID] = &copy
	s.byExternal[profile.ProviderUserID] = &copy
	return nil
}

func TestResolveInstagramInboundSyncsPlaceholderContactFromProviderProfile(t *testing.T) {
	ctx := context.Background()
	teamID := uuid.New()
	contactID := uuid.New()
	profileID := uuid.New()
	senderID := "1066988869093827"
	avatar := "https://cdn.example/lily.jpg"

	contacts := map[uuid.UUID]*Contact{
		contactID: {
			ID:           contactID,
			TeamID:       teamID,
			ContactName:  "Instagram User",
			CustomerName: "Instagram User",
			Phone:        "ig:" + senderID,
			Source:       SourceInstagram,
		},
	}
	profile := &provider.Profile{
		ID:             profileID,
		TeamID:         teamID,
		ContactID:      contactID,
		Provider:       provider.Instagram,
		ProviderUserID: senderID,
		DisplayName:    "Lily Masitha",
		Username:       "lyymst",
		AvatarURL:      avatar,
		Status:         provider.StatusActive,
	}
	providers := &igProviderRepoStub{
		byExternal: map[string]*provider.Profile{senderID: profile},
		byID:       map[uuid.UUID]*provider.Profile{profileID: profile},
	}

	service := NewService(&igContactRepoStub{contacts: contacts}, nil, nil, nil)
	service.SetProviderRepository(providers)

	// Echo/outbound often re-resolves with an empty public_profile payload; fall back to stored provider fields.
	got, err := service.ResolveInstagramInbound(ctx, teamID, senderID, "", "", "")
	if err != nil {
		t.Fatalf("ResolveInstagramInbound: %v", err)
	}
	if got.ContactName != "Lily Masitha" {
		t.Fatalf("contact_name = %q, want Lily Masitha", got.ContactName)
	}
	if got.AvatarURL == nil || *got.AvatarURL != avatar {
		t.Fatalf("avatar = %#v, want %q", got.AvatarURL, avatar)
	}
	if providers.byID[profileID].DisplayName != "Lily Masitha" {
		t.Fatal("empty fetch must not wipe provider display_name")
	}
}

func TestResolveInstagramInboundCreatesContactWithAvatar(t *testing.T) {
	ctx := context.Background()
	teamID := uuid.New()
	providers := &igProviderRepoStub{
		byExternal: map[string]*provider.Profile{},
		byID:       map[uuid.UUID]*provider.Profile{},
	}
	contacts := &igContactRepoStub{contacts: map[uuid.UUID]*Contact{}}
	service := NewService(contacts, nil, nil, nil)
	service.SetProviderRepository(providers)

	got, err := service.ResolveInstagramInbound(
		ctx, teamID, "1555989752701084", "Tommy Japola", "doktertommy", "https://cdn.example/tommy.jpg",
	)
	if err != nil {
		t.Fatalf("ResolveInstagramInbound: %v", err)
	}
	if got.ContactName != "Tommy Japola" {
		t.Fatalf("contact_name = %q", got.ContactName)
	}
	if got.AvatarURL == nil || *got.AvatarURL != "https://cdn.example/tommy.jpg" {
		t.Fatalf("avatar = %#v", got.AvatarURL)
	}
}
