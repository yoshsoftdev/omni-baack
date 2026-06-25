package meta_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	metasvc "github.com/omnichannel/omni-chan-v2/internal/meta"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type memorySessionRepo struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*metasvc.EmbeddedSignupSession
	byState  map[string]uuid.UUID
	byHash   map[string]uuid.UUID
}

func newMemorySessionRepo() *memorySessionRepo {
	return &memorySessionRepo{
		sessions: make(map[uuid.UUID]*metasvc.EmbeddedSignupSession),
		byState:  make(map[string]uuid.UUID),
		byHash:   make(map[string]uuid.UUID),
	}
}

func (r *memorySessionRepo) Create(_ context.Context, session *metasvc.EmbeddedSignupSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *session
	r.sessions[session.ID] = &cp
	r.byState[session.OAuthState] = session.ID
	return nil
}

func (r *memorySessionRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*metasvc.EmbeddedSignupSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	if !ok || s.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *memorySessionRepo) FindByOAuthState(_ context.Context, oauthState string) (*metasvc.EmbeddedSignupSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byState[oauthState]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *r.sessions[id]
	return &cp, nil
}

func (r *memorySessionRepo) FindByOAuthCodeHash(_ context.Context, codeHash string) (*metasvc.EmbeddedSignupSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byHash[codeHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *r.sessions[id]
	return &cp, nil
}

func (r *memorySessionRepo) Update(_ context.Context, session *metasvc.EmbeddedSignupSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *session
	r.sessions[session.ID] = &cp
	if session.OAuthCodeHash != nil {
		r.byHash[*session.OAuthCodeHash] = session.ID
	}
	return nil
}

type signupChannelRepo struct {
	ch    *channel.Channel
	creds *channel.Credentials
	ob    *channel.Onboarding
}

func (s *signupChannelRepo) Create(context.Context, *channel.Channel) error { return nil }
func (s *signupChannelRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*channel.Channel, error) {
	if s.ch.TeamID != teamID || s.ch.ID != id {
		return nil, repository.ErrNotFound
	}
	cp := *s.ch
	return &cp, nil
}
func (s *signupChannelRepo) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	return repository.Page[channel.Channel]{}, nil
}
func (s *signupChannelRepo) Update(_ context.Context, ch *channel.Channel) error {
	*s.ch = *ch
	return nil
}
func (s *signupChannelRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *signupChannelRepo) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s *signupChannelRepo) UpsertCredentials(_ context.Context, cred *channel.Credentials) error {
	s.creds = cred
	return nil
}
func (s *signupChannelRepo) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return s.creds, nil
}
func (s *signupChannelRepo) DeleteCredentials(context.Context, uuid.UUID) error { return nil }
func (s *signupChannelRepo) UpsertOnboarding(_ context.Context, ob *channel.Onboarding) error {
	s.ob = ob
	return nil
}
func (s *signupChannelRepo) FindOnboarding(context.Context, uuid.UUID) (*channel.Onboarding, error) {
	return s.ob, nil
}
func (s *signupChannelRepo) CountMetaReady(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s *signupChannelRepo) FindByPhoneNumberID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s *signupChannelRepo) FindByWABAID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s *signupChannelRepo) UpdateHealthMetadata(context.Context, uuid.UUID, []byte) error { return nil }

type mockMetaClient struct {
	exchange func(code string) (*metapkg.TokenResponse, error)
}

func (m *mockMetaClient) VerifyWebhook(string, string, string, string) (string, error) { return "", nil }
func (m *mockMetaClient) ListTemplates(context.Context, string, string) ([]metasvc.MessageTemplate, error) {
	return nil, nil
}
func (m *mockMetaClient) GetPhoneNumbers(context.Context, string, string) ([]metasvc.PhoneNumber, error) {
	return nil, nil
}
func (m *mockMetaClient) GetPhoneNumber(context.Context, string, string) (*metasvc.PhoneNumber, error) {
	return &metasvc.PhoneNumber{ID: "phone-1", DisplayPhoneNumber: "+123"}, nil
}
func (m *mockMetaClient) GetWABA(context.Context, string, string) (*metasvc.WABA, error) { return nil, nil }
func (m *mockMetaClient) GetBusiness(context.Context, string, string) (*metasvc.Business, error) {
	return nil, nil
}
func (m *mockMetaClient) Ping(context.Context, string) error { return nil }
func (m *mockMetaClient) ExchangeEmbeddedCode(_ context.Context, _, _, code string) (*metasvc.TokenResponse, error) {
	if m.exchange != nil {
		return m.exchange(code)
	}
	return &metasvc.TokenResponse{AccessToken: "token", ExpiresIn: 3600}, nil
}

type readinessSpy struct {
	called bool
}

func (r *readinessSpy) RefreshReadinessInternal(context.Context, uuid.UUID) error {
	r.called = true
	return nil
}

func newTestSignupService(meta *mockMetaClient, chRepo *signupChannelRepo, sessions *memorySessionRepo) *metasvc.EmbeddedSignupService {
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	return metasvc.NewEmbeddedSignupService(meta, chRepo, sessions, cipher, config.MetaConfig{
		AppID: "app", AppSecret: "secret", ConfigID: "cfg",
	}, &readinessSpy{}, nil)
}

func TestEmbeddedSignupOAuthSuccess(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	chRepo := &signupChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID, Status: channel.StatusDisconnected}}
	sessions := newMemorySessionRepo()
	spy := &readinessSpy{}
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	svc := metasvc.NewEmbeddedSignupService(&mockMetaClient{}, chRepo, sessions, cipher, config.MetaConfig{
		AppID: "app", AppSecret: "secret", ConfigID: "cfg",
	}, spy, nil)

	start, err := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	result, err := svc.CompleteEmbeddedSignup(context.Background(), teamID, channelID, start.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "auth-code", OAuthState: start.OAuthState, BusinessID: "biz-1", WABAID: "waba-1", PhoneNumberID: "phone-1",
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Status != string(channel.StatusConnected) || chRepo.ch.BusinessID == nil || *chRepo.ch.BusinessID != "biz-1" {
		t.Fatalf("channel not provisioned: %+v", chRepo.ch)
	}
	if chRepo.ob == nil || chRepo.ob.Status != channel.OnboardingCompleted {
		t.Fatal("onboarding not completed")
	}
	if !spy.called {
		t.Fatal("readiness not refreshed")
	}
}

func TestEmbeddedSignupInvalidCode(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	chRepo := &signupChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID}}
	meta := &mockMetaClient{exchange: func(string) (*metapkg.TokenResponse, error) {
		return nil, metapkg.NewAPIError(400, `{"error":{"message":"Invalid authorization code"}}`)
	}}
	svc := newTestSignupService(meta, chRepo, newMemorySessionRepo())
	start, _ := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	_, err := svc.CompleteEmbeddedSignup(context.Background(), teamID, channelID, start.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "bad", OAuthState: start.OAuthState, WABAID: "waba", PhoneNumberID: "phone",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEmbeddedSignupExpiredCode(t *testing.T) {
	meta := &mockMetaClient{exchange: func(string) (*metapkg.TokenResponse, error) {
		return nil, metapkg.NewAPIError(400, `{"error":{"message":"Session has expired"}}`)
	}}
	teamID := uuid.New()
	channelID := uuid.New()
	svc := newTestSignupService(meta, &signupChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID}}, newMemorySessionRepo())
	start, _ := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	_, err := svc.CompleteEmbeddedSignup(context.Background(), teamID, channelID, start.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "expired", OAuthState: start.OAuthState, WABAID: "waba", PhoneNumberID: "phone",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEmbeddedSignupSessionExpiration(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	sessions := newMemorySessionRepo()
	svc := newTestSignupService(&mockMetaClient{}, &signupChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID}}, sessions)
	start, _ := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	id, _ := uuid.Parse(start.SessionID)
	sessions.mu.Lock()
	sessions.sessions[id].ExpiresAt = time.Now().UTC().Add(-time.Minute)
	sessions.mu.Unlock()
	_, err := svc.CompleteEmbeddedSignup(context.Background(), teamID, channelID, start.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "code", OAuthState: start.OAuthState, WABAID: "waba", PhoneNumberID: "phone",
	})
	if err == nil {
		t.Fatal("expected expired session error")
	}
}

func TestEmbeddedSignupCrossTeamAccess(t *testing.T) {
	teamID := uuid.New()
	otherTeam := uuid.New()
	channelID := uuid.New()
	svc := newTestSignupService(&mockMetaClient{}, &signupChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID}}, newMemorySessionRepo())
	start, _ := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	_, err := svc.CompleteEmbeddedSignup(context.Background(), otherTeam, channelID, start.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "code", OAuthState: start.OAuthState, WABAID: "waba", PhoneNumberID: "phone",
	})
	if err == nil {
		t.Fatal("expected not found/forbidden")
	}
}

func TestEmbeddedSignupReplayAttack(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	chRepo := &signupChannelRepo{ch: &channel.Channel{ID: channelID, TeamID: teamID}}
	svc := newTestSignupService(&mockMetaClient{}, chRepo, newMemorySessionRepo())
	start1, _ := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	_, err := svc.CompleteEmbeddedSignup(context.Background(), teamID, channelID, start1.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "same-code", OAuthState: start1.OAuthState, WABAID: "waba", PhoneNumberID: "phone",
	})
	if err != nil {
		t.Fatalf("first complete: %v", err)
	}
	start2, _ := svc.StartEmbeddedSignup(context.Background(), teamID, channelID)
	_, err = svc.CompleteEmbeddedSignup(context.Background(), teamID, channelID, start2.SessionID, metasvc.CompleteEmbeddedSignupInput{
		Code: "same-code", OAuthState: start2.OAuthState, WABAID: "waba", PhoneNumberID: "phone",
	})
	if err == nil {
		t.Fatal("expected replay error")
	}
}
