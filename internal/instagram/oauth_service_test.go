package instagram

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type stubCipher struct{}

func (stubCipher) Encrypt(plaintext string) ([]byte, error) { return []byte("enc:" + plaintext), nil }

type stubLoginClient struct {
	profile *metaclient.InstagramUser
}

func (s *stubLoginClient) ExchangeInstagramCode(context.Context, string, string, string, string) (*metaclient.InstagramCodeExchangeResponse, error) {
	return &metaclient.InstagramCodeExchangeResponse{AccessToken: "short", UserID: "123"}, nil
}

func (s *stubLoginClient) ExchangeInstagramLongLivedToken(context.Context, string, string) (*metaclient.TokenResponse, error) {
	return &metaclient.TokenResponse{AccessToken: "long", ExpiresIn: 5184000}, nil
}

func (s *stubLoginClient) GetInstagramUser(context.Context, string) (*metaclient.InstagramUser, error) {
	return s.profile, nil
}

func (s *stubLoginClient) SubscribeInstagramApp(context.Context, string, string, string) error {
	return nil
}

type memChannels struct {
	byID map[uuid.UUID]*channel.Channel
	byIG map[string]*channel.Channel
}

func newMemChannels() *memChannels {
	return &memChannels{byID: map[uuid.UUID]*channel.Channel{}, byIG: map[string]*channel.Channel{}}
}

func (m *memChannels) Create(_ context.Context, ch *channel.Channel) error {
	cp := *ch
	m.byID[ch.ID] = &cp
	if ch.IGUserID != nil {
		m.byIG[*ch.IGUserID] = &cp
	}
	return nil
}

func (m *memChannels) Update(_ context.Context, ch *channel.Channel) error {
	cp := *ch
	m.byID[ch.ID] = &cp
	if ch.IGUserID != nil {
		m.byIG[*ch.IGUserID] = &cp
	}
	return nil
}

func (m *memChannels) SoftDelete(_ context.Context, teamID, id uuid.UUID) error {
	ch, ok := m.byID[id]
	if !ok || ch.TeamID != teamID {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	ch.DeletedAt = &now
	ch.Status = channel.StatusDisconnected
	return nil
}

func (m *memChannels) FindByIGUserID(_ context.Context, teamID uuid.UUID, igUserID string) (*channel.Channel, error) {
	ch, ok := m.byIG[igUserID]
	if !ok || ch.TeamID != teamID || ch.DeletedAt != nil {
		return nil, repository.ErrNotFound
	}
	cp := *ch
	return &cp, nil
}

func (m *memChannels) FindConnectedByType(_ context.Context, teamID uuid.UUID, channelType channel.ChannelType) (*channel.Channel, error) {
	for _, ch := range m.byID {
		if ch.TeamID == teamID && ch.ChannelType == channelType && ch.Status == channel.StatusConnected && ch.DeletedAt == nil {
			cp := *ch
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *memChannels) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return nil, repository.ErrNotFound
}

func (m *memChannels) UpsertCredentials(context.Context, *channel.Credentials) error { return nil }
func (m *memChannels) DeleteCredentials(context.Context, uuid.UUID) error            { return nil }
func (m *memChannels) UpsertOnboarding(context.Context, *channel.Onboarding) error   { return nil }

func TestHandleCallback_CreatesInstagramChannel(t *testing.T) {
	store := newMemChannels()
	svc := NewService(store, nil, nil, stubCipher{}, &stubLoginClient{
		profile: &metaclient.InstagramUser{
			UserID:      "178414000",
			Username:    "brand.ig",
			AccountType: "BUSINESS",
		},
	}, config.InstagramConfig{
		AppID:       "app",
		AppSecret:   "secret",
		RedirectURI: "https://api.example/api/v1/channels/instagram/callback",
	})

	teamID := uuid.New()
	userID := uuid.New()
	state, _, err := issueOAuthState("secret", teamID, userID, "nonce", "", oauthStateTTL)
	if err != nil {
		t.Fatal(err)
	}

	result, returnURL, err := svc.HandleCallback(context.Background(), "auth-code", state)
	if err != nil {
		t.Fatal(err)
	}
	if returnURL != "" {
		t.Fatalf("unexpected return url: %s", returnURL)
	}
	if !result.Connected || result.IGUserID != "178414000" || result.Username != "brand.ig" {
		t.Fatalf("unexpected result: %+v", result)
	}

	ch, err := store.FindConnectedByType(context.Background(), teamID, channel.TypeInstagram)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Provider != "instagram" || ch.ChannelType != channel.TypeInstagram {
		t.Fatalf("unexpected channel: %+v", ch)
	}
}

func TestHandleCallback_RejectsPersonalAccount(t *testing.T) {
	svc := NewService(newMemChannels(), nil, nil, stubCipher{}, &stubLoginClient{
		profile: &metaclient.InstagramUser{
			UserID:      "178414000",
			Username:    "personal.ig",
			AccountType: "PERSONAL",
		},
	}, config.InstagramConfig{
		AppID:       "app",
		AppSecret:   "secret",
		RedirectURI: "https://api.example/callback",
	})

	teamID := uuid.New()
	userID := uuid.New()
	state, _, err := issueOAuthState("secret", teamID, userID, "nonce", "", oauthStateTTL)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.HandleCallback(context.Background(), "auth-code", state)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
