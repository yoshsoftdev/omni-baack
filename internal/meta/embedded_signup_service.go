package meta

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// WorkspaceReadinessRefresher recomputes workspace readiness after signup.
type WorkspaceReadinessRefresher interface {
	RefreshReadinessInternal(ctx context.Context, teamID uuid.UUID) error
}

// PermissionRefresher updates channel sync permissions after OAuth reconnect.
type PermissionRefresher interface {
	RefreshPermissionsAfterConnect(ctx context.Context, teamID, channelID uuid.UUID, accessToken string) error
}

type EmbeddedSignupService struct {
	meta                MetaClient
	channels            channel.Repository
	sessions            EmbeddedSignupSessionRepository
	cipher              *cryptoutil.TokenCipher
	cfg                 config.MetaConfig
	readiness           WorkspaceReadinessRefresher
	permissionRefresher PermissionRefresher
	log                 *slog.Logger
}

func NewEmbeddedSignupService(
	meta MetaClient,
	channels channel.Repository,
	sessions EmbeddedSignupSessionRepository,
	cipher *cryptoutil.TokenCipher,
	cfg config.MetaConfig,
	readiness WorkspaceReadinessRefresher,
	log *slog.Logger,
) *EmbeddedSignupService {
	if log == nil {
		log = slog.Default()
	}
	return &EmbeddedSignupService{
		meta:      meta,
		channels:  channels,
		sessions:  sessions,
		cipher:    cipher,
		cfg:       cfg,
		readiness: readiness,
		log:       log,
	}
}

func (s *EmbeddedSignupService) SetPermissionRefresher(refresher PermissionRefresher) {
	s.permissionRefresher = refresher
}

type StartEmbeddedSignupResult struct {
	SessionID  string    `json:"session_id"`
	OAuthState string    `json:"oauth_state"`
	Status     string    `json:"status"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (s *EmbeddedSignupService) StartEmbeddedSignup(ctx context.Context, teamID, channelID uuid.UUID) (*StartEmbeddedSignupResult, error) {
	if _, err := s.channels.FindByID(ctx, teamID, channelID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if s.cfg.AppID == "" || s.cfg.ConfigID == "" {
		return nil, serviceerrors.NewValidation("meta", "embedded signup is not configured")
	}
	if s.sessions == nil {
		return nil, serviceerrors.NewValidation("meta", "embedded signup sessions are not configured")
	}

	oauthState, err := generateOAuthState()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &EmbeddedSignupSession{
		ID:         uuid.New(),
		TeamID:     teamID,
		ChannelID:  channelID,
		Status:     EmbeddedSignupPending,
		OAuthState: oauthState,
		CreatedAt:  now,
		ExpiresAt:  now.Add(defaultSessionTTL),
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	return &StartEmbeddedSignupResult{
		SessionID:  session.ID.String(),
		OAuthState: oauthState,
		Status:     string(EmbeddedSignupPending),
		ExpiresAt:  session.ExpiresAt,
	}, nil
}

type EmbeddedSignupStatusResult struct {
	SessionID     string     `json:"session_id"`
	Status        string     `json:"status"`
	BusinessID    string     `json:"business_id,omitempty"`
	WABAID        string     `json:"waba_id,omitempty"`
	PhoneNumberID string     `json:"phone_number_id,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

func (s *EmbeddedSignupService) GetEmbeddedSignupStatus(ctx context.Context, teamID, channelID uuid.UUID, sessionID string) (*EmbeddedSignupStatusResult, error) {
	session, err := s.loadOwnedSession(ctx, teamID, channelID, sessionID)
	if err != nil {
		return nil, err
	}
	s.expireIfNeeded(ctx, session)

	businessID := ""
	if session.BusinessID != nil {
		businessID = *session.BusinessID
	}
	wabaID := ""
	phoneID := ""
	if session.Status == EmbeddedSignupCompleted {
		if ch, err := s.channels.FindByID(ctx, teamID, channelID); err == nil {
			if ch.WABAID != nil {
				wabaID = *ch.WABAID
			}
			if ch.PhoneNumberID != nil {
				phoneID = *ch.PhoneNumberID
			}
			if ch.BusinessID != nil && businessID == "" {
				businessID = *ch.BusinessID
			}
		}
	}
	errMsg := ""
	if session.ErrorMessage != nil {
		errMsg = *session.ErrorMessage
	}
	return &EmbeddedSignupStatusResult{
		SessionID:     session.ID.String(),
		Status:        string(session.Status),
		BusinessID:    businessID,
		WABAID:        wabaID,
		PhoneNumberID: phoneID,
		ErrorMessage:  errMsg,
		ExpiresAt:     session.ExpiresAt,
		CompletedAt:   session.CompletedAt,
	}, nil
}

type CompleteEmbeddedSignupInput struct {
	Code          string
	OAuthState    string
	BusinessID    string
	WABAID        string
	PhoneNumberID string
}

type CompleteEmbeddedSignupResult struct {
	ChannelID     uuid.UUID `json:"channel_id"`
	Status        string    `json:"status"`
	BusinessID    string    `json:"business_id,omitempty"`
	WABAID        string    `json:"waba_id,omitempty"`
	PhoneNumberID string    `json:"phone_number_id,omitempty"`
	PhoneNumber   string    `json:"phone_number,omitempty"`
}

func (s *EmbeddedSignupService) CompleteEmbeddedSignup(
	ctx context.Context,
	teamID, channelID uuid.UUID,
	sessionID string,
	input CompleteEmbeddedSignupInput,
) (*CompleteEmbeddedSignupResult, error) {
	session, err := s.loadOwnedSession(ctx, teamID, channelID, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSessionActive(session); err != nil {
		return nil, err
	}
	if state := strings.TrimSpace(input.OAuthState); state != "" && state != session.OAuthState {
		return nil, serviceerrors.NewValidation("oauth_state", "invalid oauth state")
	}

	if s.cfg.AppID == "" || s.cfg.AppSecret == "" {
		return nil, serviceerrors.NewValidation("meta", "embedded signup is not configured")
	}

	code := strings.TrimSpace(input.Code)
	wabaID := strings.TrimSpace(input.WABAID)
	phoneNumberID := strings.TrimSpace(input.PhoneNumberID)
	businessID := strings.TrimSpace(input.BusinessID)
	if code == "" || wabaID == "" || phoneNumberID == "" {
		return nil, serviceerrors.NewValidation("input", "code, waba_id, and phone_number_id are required")
	}

	codeHash := hashOAuthCode(code)
	if existing, err := s.sessions.FindByOAuthCodeHash(ctx, codeHash); err == nil && existing != nil && existing.ID != session.ID {
		return nil, serviceerrors.NewValidation("code", "authorization code has already been used")
	}

	tokenResp, err := s.meta.ExchangeEmbeddedCode(ctx, s.cfg.AppID, s.cfg.AppSecret, code)
	if err != nil {
		s.markFailed(ctx, session, oauthErrorMessage(err))
		return nil, mapOAuthError(err)
	}
	if _, err := s.meta.GetPhoneNumber(ctx, phoneNumberID, tokenResp.AccessToken); err != nil {
		s.markFailed(ctx, session, "phone number is not accessible")
		return nil, serviceerrors.NewValidation("phone_number_id", "invalid phone number")
	}

	ch, err := s.channels.FindByID(ctx, teamID, session.ChannelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	now := time.Now().UTC()
	display := phoneNumberID
	if phone, err := s.meta.GetPhoneNumber(ctx, phoneNumberID, tokenResp.AccessToken); err == nil && phone.DisplayPhoneNumber != "" {
		display = phone.DisplayPhoneNumber
	}

	connType := channel.ConnectionEmbeddedCoex
	ch.Status = channel.StatusConnected
	ch.ConnectionType = &connType
	ch.WABAID = &wabaID
	ch.PhoneNumberID = &phoneNumberID
	ch.PhoneNumber = &display
	ch.IsCoex = true
	ch.LastConnectedAt = &now
	ch.UpdatedAt = now
	if businessID != "" {
		ch.BusinessID = &businessID
	}
	if err := s.channels.Update(ctx, ch); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	if s.cipher == nil {
		return nil, errors.New("token encryption is not configured")
	}
	enc, err := s.cipher.Encrypt(tokenResp.AccessToken)
	if err != nil {
		return nil, err
	}
	var expiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		exp := now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		expiresAt = &exp
	}
	if err := s.channels.UpsertCredentials(ctx, &channel.Credentials{
		ChannelID:      ch.ID,
		AccessTokenEnc: enc,
		TokenExpiresAt: expiresAt,
		UpdatedAt:      now,
	}); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	if s.permissionRefresher != nil {
		if err := s.permissionRefresher.RefreshPermissionsAfterConnect(ctx, teamID, ch.ID, tokenResp.AccessToken); err != nil {
			s.log.Warn("embedded_signup_permission_refresh_failed",
				"event", "embedded_signup_permission_refresh_failed",
				"team_id", teamID.String(),
				"channel_id", channelID.String(),
				"error", err.Error(),
			)
		}
	}

	ob := &channel.Onboarding{ChannelID: ch.ID, Status: channel.OnboardingCompleted, CompletedAt: &now, UpdatedAt: now}
	if err := s.channels.UpsertOnboarding(ctx, ob); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	session.Status = EmbeddedSignupCompleted
	session.OAuthCodeHash = &codeHash
	session.CompletedAt = &now
	if businessID != "" {
		session.BusinessID = &businessID
	}
	if err := s.sessions.Update(ctx, session); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	if s.readiness != nil {
		if err := s.readiness.RefreshReadinessInternal(ctx, teamID); err != nil {
			s.log.Warn("embedded_signup_readiness_refresh_failed",
				"event", "embedded_signup_readiness_refresh_failed",
				"team_id", teamID.String(),
				"channel_id", channelID.String(),
				"error", err.Error(),
			)
		}
	}

	s.log.Info("embedded_signup_completed",
		"event", "embedded_signup_completed",
		"team_id", teamID.String(),
		"channel_id", channelID.String(),
		"session_id", session.ID.String(),
	)

	return &CompleteEmbeddedSignupResult{
		ChannelID:     ch.ID,
		Status:        string(ch.Status),
		BusinessID:    businessID,
		WABAID:        wabaID,
		PhoneNumberID: phoneNumberID,
		PhoneNumber:   display,
	}, nil
}

func (s *EmbeddedSignupService) loadOwnedSession(ctx context.Context, teamID, channelID uuid.UUID, sessionID string) (*EmbeddedSignupSession, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, serviceerrors.NewValidation("session_id", "invalid session id")
	}
	session, err := s.sessions.FindByID(ctx, teamID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, serviceerrors.ErrNotFound
		}
		return nil, serviceerrors.MapRepository(err)
	}
	if session.ChannelID != channelID {
		return nil, serviceerrors.ErrNotFound
	}
	return session, nil
}

func (s *EmbeddedSignupService) ensureSessionActive(session *EmbeddedSignupSession) error {
	if session.Status == EmbeddedSignupCompleted {
		return serviceerrors.NewValidation("session_id", "embedded signup session already completed")
	}
	if session.Status == EmbeddedSignupFailed {
		return serviceerrors.NewValidation("session_id", "embedded signup session failed")
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return serviceerrors.NewValidation("session_id", "embedded signup session expired")
	}
	return nil
}

func (s *EmbeddedSignupService) expireIfNeeded(ctx context.Context, session *EmbeddedSignupSession) {
	if session.Status != EmbeddedSignupPending {
		return
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		session.Status = EmbeddedSignupExpired
		_ = s.sessions.Update(ctx, session)
	}
}

func (s *EmbeddedSignupService) markFailed(ctx context.Context, session *EmbeddedSignupSession, message string) {
	session.Status = EmbeddedSignupFailed
	session.ErrorMessage = &message
	_ = s.sessions.Update(ctx, session)
}

func generateOAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashOAuthCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func mapOAuthError(err error) error {
	switch metapkg.ClassifyOAuthError(err) {
	case metapkg.OAuthErrorExpired:
		return serviceerrors.NewValidation("code", "authorization code expired")
	case metapkg.OAuthErrorInvalid:
		return serviceerrors.NewValidation("code", "invalid authorization code")
	default:
		return serviceerrors.NewValidation("code", "invalid authorization code")
	}
}

func oauthErrorMessage(err error) string {
	switch metapkg.ClassifyOAuthError(err) {
	case metapkg.OAuthErrorExpired:
		return "authorization code expired"
	case metapkg.OAuthErrorInvalid:
		return "invalid authorization code"
	default:
		return "failed to exchange authorization code"
	}
}
