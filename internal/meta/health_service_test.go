package meta_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	metasvc "github.com/omnichannel/omni-chan-v2/internal/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type stubChannelRepo struct {
	ch *channel.Channel
}

func (s stubChannelRepo) Create(context.Context, *channel.Channel) error { return nil }
func (s stubChannelRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*channel.Channel, error) {
	return s.ch, nil
}
func (s stubChannelRepo) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	return repository.Page[channel.Channel]{}, nil
}
func (s stubChannelRepo) Update(context.Context, *channel.Channel) error { return nil }
func (s stubChannelRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s stubChannelRepo) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s stubChannelRepo) UpsertCredentials(context.Context, *channel.Credentials) error { return nil }
func (s stubChannelRepo) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) DeleteCredentials(context.Context, uuid.UUID) error { return nil }
func (s stubChannelRepo) UpsertOnboarding(context.Context, *channel.Onboarding) error { return nil }
func (s stubChannelRepo) FindOnboarding(context.Context, uuid.UUID) (*channel.Onboarding, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) CountMetaReady(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s stubChannelRepo) FindByPhoneNumberID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindByWABAID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindByIGUserID(context.Context, uuid.UUID, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindByIGUserIDGlobal(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindConnectedByType(context.Context, uuid.UUID, channel.ChannelType) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) UpdateHealthMetadata(context.Context, uuid.UUID, []byte) error { return nil }

func TestHealthServiceMissingWABA(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	chRepo := stubChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID}}
	health := metasvc.NewHealthService(nil, nil, chRepo, cipher, nil, nil)
	result, err := health.CheckChannel(context.Background(), teamID, channelID)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if result.Healthy {
		t.Fatal("expected unhealthy")
	}
}
