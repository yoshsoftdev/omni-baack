package campaign

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type channelRepoStub struct {
	ch *channel.Channel
	ob *channel.Onboarding
}

func (s channelRepoStub) Create(context.Context, *channel.Channel) error { return nil }
func (s channelRepoStub) FindByID(context.Context, uuid.UUID, uuid.UUID) (*channel.Channel, error) {
	if s.ch == nil {
		return nil, repository.ErrNotFound
	}
	return s.ch, nil
}
func (s channelRepoStub) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	return repository.Page[channel.Channel]{}, nil
}
func (s channelRepoStub) Update(context.Context, *channel.Channel) error { return nil }
func (s channelRepoStub) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s channelRepoStub) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s channelRepoStub) UpsertCredentials(context.Context, *channel.Credentials) error {
	return nil
}
func (s channelRepoStub) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return nil, repository.ErrNotFound
}
func (s channelRepoStub) DeleteCredentials(context.Context, uuid.UUID) error { return nil }
func (s channelRepoStub) UpsertOnboarding(context.Context, *channel.Onboarding) error { return nil }
func (s channelRepoStub) FindOnboarding(context.Context, uuid.UUID) (*channel.Onboarding, error) {
	if s.ob == nil {
		return nil, repository.ErrNotFound
	}
	return s.ob, nil
}
func (s channelRepoStub) CountMetaReady(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s channelRepoStub) FindByPhoneNumberID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s channelRepoStub) FindByWABAID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s channelRepoStub) FindByIGUserID(context.Context, uuid.UUID, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s channelRepoStub) FindByIGUserIDGlobal(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s channelRepoStub) FindConnectedByType(context.Context, uuid.UUID, channel.ChannelType) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s channelRepoStub) UpdateHealthMetadata(context.Context, uuid.UUID, []byte) error { return nil }

func TestValidateChannelForCampaignRejectsInstagram(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	repo := channelRepoStub{
		ch: &channel.Channel{
			ID:          channelID,
			TeamID:      teamID,
			ChannelType: channel.TypeInstagram,
			Status:      channel.StatusConnected,
		},
		ob: &channel.Onboarding{Status: channel.OnboardingCompleted},
	}
	err := validateChannelForCampaign(context.Background(), repo, teamID, channelID)
	if err == nil || !serviceerrors.IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestValidateChannelForCampaignAllowsWhatsApp(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	repo := channelRepoStub{
		ch: &channel.Channel{
			ID:          channelID,
			TeamID:      teamID,
			ChannelType: channel.TypeWhatsApp,
			Status:      channel.StatusConnected,
		},
		ob: &channel.Onboarding{Status: channel.OnboardingCompleted},
	}
	if err := validateChannelForCampaign(context.Background(), repo, teamID, channelID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
