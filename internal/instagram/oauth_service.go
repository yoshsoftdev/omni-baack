package instagram

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

const oauthStateTTL = time.Hour

type TokenCipher interface {
	Encrypt(plaintext string) ([]byte, error)
}

type LoginClient interface {
	ExchangeInstagramCode(ctx context.Context, appID, appSecret, redirectURI, code string) (*metaclient.InstagramCodeExchangeResponse, error)
	ExchangeInstagramLongLivedToken(ctx context.Context, appSecret, shortLivedToken string) (*metaclient.TokenResponse, error)
	GetInstagramUser(ctx context.Context, accessToken string) (*metaclient.InstagramUser, error)
	SubscribeInstagramApp(ctx context.Context, igUserID, accessToken string, fields string) error
}

type ChannelStore interface {
	Create(ctx context.Context, ch *channel.Channel) error
	Update(ctx context.Context, ch *channel.Channel) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
	FindByIGUserID(ctx context.Context, teamID uuid.UUID, igUserID string) (*channel.Channel, error)
	FindConnectedByType(ctx context.Context, teamID uuid.UUID, channelType channel.ChannelType) (*channel.Channel, error)
	FindCredentials(ctx context.Context, channelID uuid.UUID) (*channel.Credentials, error)
	UpsertCredentials(ctx context.Context, cred *channel.Credentials) error
	DeleteCredentials(ctx context.Context, channelID uuid.UUID) error
	UpsertOnboarding(ctx context.Context, ob *channel.Onboarding) error
}

type Service struct {
	channels ChannelStore
	teams    team.Repository
	rbac     *rbac.Service
	cipher   TokenCipher
	meta     LoginClient
	cfg      config.InstagramConfig
}

func NewService(
	channels ChannelStore,
	teams team.Repository,
	rbacSvc *rbac.Service,
	cipher TokenCipher,
	meta LoginClient,
	cfg config.InstagramConfig,
) *Service {
	return &Service{
		channels: channels,
		teams:    teams,
		rbac:     rbacSvc,
		cipher:   cipher,
		meta:     meta,
		cfg:      cfg,
	}
}

type ConnectURLResult struct {
	AuthorizationURL string    `json:"authorization_url"`
	State            string    `json:"state"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type StatusResult struct {
	Connected   bool       `json:"connected"`
	Username    string     `json:"username,omitempty"`
	AccountType string     `json:"account_type,omitempty"`
	TokenExpiry *time.Time `json:"token_expiry,omitempty"`
}

type CallbackResult struct {
	ChannelID   uuid.UUID `json:"channel_id"`
	Connected   bool      `json:"connected"`
	Username    string    `json:"username"`
	AccountType string    `json:"account_type"`
	IGUserID    string    `json:"instagram_user_id"`
}

type oauthStateClaims struct {
	TeamID    uuid.UUID `json:"tid"`
	UserID    uuid.UUID `json:"uid"`
	Nonce     string    `json:"n"`
	ReturnURL string    `json:"ru,omitempty"`
	ExpiresAt int64     `json:"exp"`
}

type channelHealthMetadata struct {
	AccountType string `json:"account_type"`
	Username    string `json:"username"`
}

func (s *Service) authorizeManage(ctx context.Context, teamID, userID uuid.UUID, actorRole string) error {
	if s.rbac != nil && !s.rbac.Require(actorRole, rbac.PermDeviceManage) {
		return serviceerrors.ErrForbidden
	}
	if s.teams == nil {
		return nil
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		if errors.Is(err, team.ErrMemberNotFound) {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) authorizeView(ctx context.Context, teamID, userID uuid.UUID, actorRole string) error {
	if s.rbac != nil && !s.rbac.Require(actorRole, rbac.PermDeviceView) {
		return serviceerrors.ErrForbidden
	}
	if s.teams == nil {
		return nil
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		if errors.Is(err, team.ErrMemberNotFound) {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) ConnectURL(ctx context.Context, teamID, userID uuid.UUID, actorRole, returnURL string) (*ConnectURLResult, error) {
	if err := s.authorizeManage(ctx, teamID, userID, actorRole); err != nil {
		return nil, err
	}
	if err := s.validateConfig(); err != nil {
		return nil, err
	}

	nonce, err := randomNonce(16)
	if err != nil {
		return nil, err
	}
	state, expiresAt, err := issueOAuthState(s.cfg.AppSecret, teamID, userID, nonce, strings.TrimSpace(returnURL), oauthStateTTL)
	if err != nil {
		return nil, err
	}
	authURL := metaclient.BuildInstagramAuthorizeURL(s.cfg.AppID, s.cfg.RedirectURI, state, metaclient.InstagramBusinessLoginScopes)
	return &ConnectURLResult{
		AuthorizationURL: authURL,
		State:            state,
		ExpiresAt:        expiresAt,
	}, nil
}

func (s *Service) HandleCallback(ctx context.Context, code, state string) (*CallbackResult, string, error) {
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	if code == "" || state == "" {
		return nil, "", serviceerrors.NewValidation("oauth", "code and state are required")
	}
	if err := s.validateConfig(); err != nil {
		return nil, "", err
	}

	claims, err := parseOAuthState(s.cfg.AppSecret, state)
	if err != nil {
		return nil, "", serviceerrors.NewValidation("state", err.Error())
	}

	result, err := s.completeLogin(ctx, claims.TeamID, claims.UserID, code)
	if err != nil {
		return nil, claims.ReturnURL, err
	}
	return result, claims.ReturnURL, nil
}

func (s *Service) Status(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*StatusResult, error) {
	if err := s.authorizeView(ctx, teamID, userID, actorRole); err != nil {
		return nil, err
	}
	ch, err := s.channels.FindConnectedByType(ctx, teamID, channel.TypeInstagram)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &StatusResult{Connected: false}, nil
		}
		return nil, serviceerrors.MapRepository(err)
	}

	status := &StatusResult{
		Connected: true,
		Username:  strings.TrimSpace(ch.Name),
	}
	if meta := decodeHealthMetadata(ch.HealthMetadata); meta != nil {
		if status.Username == "" {
			status.Username = meta.Username
		}
		status.AccountType = meta.AccountType
	}
	creds, err := s.channels.FindCredentials(ctx, ch.ID)
	if err == nil && creds != nil {
		status.TokenExpiry = creds.TokenExpiresAt
	}
	return status, nil
}

func (s *Service) Disconnect(ctx context.Context, teamID, userID uuid.UUID, actorRole string) error {
	if err := s.authorizeManage(ctx, teamID, userID, actorRole); err != nil {
		return err
	}
	ch, err := s.channels.FindConnectedByType(ctx, teamID, channel.TypeInstagram)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return serviceerrors.MapRepository(err)
	}
	_ = s.channels.DeleteCredentials(ctx, ch.ID)
	return serviceerrors.MapRepository(s.channels.SoftDelete(ctx, teamID, ch.ID))
}

func (s *Service) completeLogin(ctx context.Context, teamID, userID uuid.UUID, code string) (*CallbackResult, error) {
	_ = userID

	slog.Info("instagram oauth: complete login started",
		"team_id", teamID.String(),
		"redirect_uri", s.cfg.RedirectURI,
		"app_id", s.cfg.AppID,
		"code_len", len(strings.TrimSpace(code)),
	)

	shortToken, err := s.meta.ExchangeInstagramCode(ctx, s.cfg.AppID, s.cfg.AppSecret, s.cfg.RedirectURI, code)
	if err != nil {
		slog.Error("instagram oauth: code exchange failed", "error", err.Error(), "team_id", teamID.String())
		return nil, fmt.Errorf("exchange instagram code: %w", err)
	}

	accessToken := strings.TrimSpace(shortToken.AccessToken)
	longToken, err := s.meta.ExchangeInstagramLongLivedToken(ctx, s.cfg.AppSecret, accessToken)
	if err != nil {
		slog.Warn("instagram oauth: long-lived exchange failed, using short-lived token", "error", err.Error())
		longToken = &metaclient.TokenResponse{AccessToken: accessToken, ExpiresIn: shortTokenExpiresIn(shortToken)}
	} else {
		accessToken = strings.TrimSpace(longToken.AccessToken)
	}

	profile, err := s.meta.GetInstagramUser(ctx, accessToken)
	if err != nil {
		slog.Error("instagram oauth: profile fetch failed", "error", err.Error(), "team_id", teamID.String())
		return nil, fmt.Errorf("load instagram account: %w", err)
	}
	slog.Info("instagram oauth: profile loaded",
		"user_id", profile.UserID,
		"username", profile.Username,
		"account_type", profile.AccountType,
		"team_id", teamID.String(),
	)
	if err := metaclient.ValidateInstagramAccountType(profile.AccountType); err != nil {
		return nil, serviceerrors.NewValidation("instagram", err.Error())
	}

	username := strings.TrimSpace(profile.Username)
	if username == "" {
		username = strings.TrimSpace(profile.Name)
	}
	if username == "" {
		username = "Instagram Business"
	}

	igUserID := strings.TrimSpace(profile.UserID)
	if igUserID == "" {
		return nil, serviceerrors.NewValidation("instagram", "instagram user id is missing from profile response")
	}

	enc, err := s.cipher.Encrypt(accessToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := metaclient.TokenExpiryFromExpiresIn(longToken.ExpiresIn)
	connType := channel.ConnectionOAuth
	healthMeta, err := json.Marshal(channelHealthMetadata{
		AccountType: strings.ToUpper(strings.TrimSpace(profile.AccountType)),
		Username:    username,
	})
	if err != nil {
		return nil, err
	}

	existing, findErr := s.channels.FindByIGUserID(ctx, teamID, igUserID)
	var ch *channel.Channel
	if findErr == nil && existing != nil {
		ch = existing
		ch.Name = username
		ch.Provider = "instagram"
		ch.ChannelType = channel.TypeInstagram
		ch.Status = channel.StatusConnected
		ch.ConnectionType = &connType
		ch.IGUserID = &igUserID
		ch.HealthMetadata = healthMeta
		ch.LastConnectedAt = &now
		ch.UpdatedAt = now
		if err := s.channels.Update(ctx, ch); err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
	} else if findErr != nil && !errors.Is(findErr, repository.ErrNotFound) {
		return nil, serviceerrors.MapRepository(findErr)
	} else {
		ch = &channel.Channel{
			ID:              uuid.New(),
			TeamID:          teamID,
			Name:            username,
			Provider:        "instagram",
			ChannelType:     channel.TypeInstagram,
			Status:          channel.StatusConnected,
			ConnectionType:  &connType,
			IGUserID:        &igUserID,
			HealthMetadata:  healthMeta,
			LastConnectedAt: &now,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.channels.Create(ctx, ch); err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
	}

	if err := s.channels.UpsertCredentials(ctx, &channel.Credentials{
		ChannelID:      ch.ID,
		AccessTokenEnc: enc,
		TokenExpiresAt: expiresAt,
		UpdatedAt:      now,
	}); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if err := s.channels.UpsertOnboarding(ctx, &channel.Onboarding{
		ChannelID:   ch.ID,
		Status:      channel.OnboardingCompleted,
		CompletedAt: &now,
		UpdatedAt:   now,
	}); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	s.ensureWebhookSubscribed(ctx, teamID, ch.ID, igUserID, accessToken)

	return &CallbackResult{
		ChannelID:   ch.ID,
		Connected:   true,
		Username:    username,
		AccountType: strings.ToUpper(strings.TrimSpace(profile.AccountType)),
		IGUserID:    igUserID,
	}, nil
}

func (s *Service) ensureWebhookSubscribed(ctx context.Context, teamID, channelID uuid.UUID, igUserID, accessToken string) {
	if s.meta == nil {
		return
	}
	if err := s.meta.SubscribeInstagramApp(ctx, igUserID, accessToken, metaclient.InstagramWebhookSubscribeFields); err != nil {
		if metaclient.IsSubscribeAlreadyDone(err) {
			slog.Info("instagram oauth: app already subscribed to webhooks",
				"team_id", teamID.String(),
				"channel_id", channelID.String(),
				"ig_user_id", igUserID,
			)
			return
		}
		// Channel is already saved; mirror WhatsApp embedded signup behavior and only warn.
		slog.Warn("instagram oauth: webhook subscribe failed",
			"team_id", teamID.String(),
			"channel_id", channelID.String(),
			"ig_user_id", igUserID,
			"error", err.Error(),
		)
		return
	}
	slog.Info("instagram oauth: app subscribed to webhooks",
		"team_id", teamID.String(),
		"channel_id", channelID.String(),
		"ig_user_id", igUserID,
	)
}

func (s *Service) validateConfig() error {
	if strings.TrimSpace(s.cfg.AppID) == "" || strings.TrimSpace(s.cfg.AppSecret) == "" {
		return serviceerrors.NewValidation("instagram", "INSTAGRAM_APP_ID and INSTAGRAM_APP_SECRET are required")
	}
	if strings.TrimSpace(s.cfg.RedirectURI) == "" {
		return serviceerrors.NewValidation("instagram", "INSTAGRAM_REDIRECT_URI is required")
	}
	return nil
}

func decodeHealthMetadata(raw []byte) *channelHealthMetadata {
	if len(raw) == 0 {
		return nil
	}
	var meta channelHealthMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	return &meta
}

func shortTokenExpiresIn(_ *metaclient.InstagramCodeExchangeResponse) int {
	return 3600
}

func randomNonce(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func issueOAuthState(secret string, teamID, userID uuid.UUID, nonce, returnURL string, ttl time.Duration) (string, time.Time, error) {
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, errors.New("oauth state secret is not configured")
	}
	if returnURL != "" {
		if _, err := url.Parse(returnURL); err != nil {
			return "", time.Time{}, fmt.Errorf("invalid return url")
		}
	}
	expiresAt := time.Now().UTC().Add(ttl)
	claims := oauthStateClaims{
		TeamID:    teamID,
		UserID:    userID,
		Nonce:     nonce,
		ReturnURL: returnURL,
		ExpiresAt: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, expiresAt, nil
}

func parseOAuthState(secret, token string) (*oauthStateClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid oauth state")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return nil, fmt.Errorf("invalid oauth state signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid oauth state payload")
	}
	var claims oauthStateClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("invalid oauth state claims")
	}
	if time.Now().UTC().After(time.Unix(claims.ExpiresAt, 0)) {
		return nil, fmt.Errorf("oauth state expired")
	}
	return &claims, nil
}
