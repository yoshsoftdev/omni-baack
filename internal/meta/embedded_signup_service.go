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

// StartTeamEmbeddedSignup begins signup without writing channel, credentials, or onboarding rows.
// The returned session_id is a signed, stateless token (no database write).
func (s *EmbeddedSignupService) StartTeamEmbeddedSignup(ctx context.Context, teamID, userID uuid.UUID) (*StartEmbeddedSignupResult, error) {
	if s.cfg.AppID == "" || s.cfg.ConfigID == "" || s.cfg.AppSecret == "" {
		return nil, serviceerrors.NewValidation("meta", "embedded signup is not configured")
	}

	oauthState, err := generateOAuthState()
	if err != nil {
		return nil, err
	}

	token, expiresAt, err := issueSignupStartToken(s.cfg.AppSecret, teamID, userID, oauthState, defaultSessionTTL)
	if err != nil {
		return nil, err
	}

	return &StartEmbeddedSignupResult{
		SessionID:  token,
		OAuthState: oauthState,
		Status:     string(EmbeddedSignupPending),
		ExpiresAt:  expiresAt,
	}, nil
}

// StartEmbeddedSignup starts re-authorization for an existing channel (session persisted in DB).
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
		ChannelID:  &channelID,
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

func (s *EmbeddedSignupService) GetTeamEmbeddedSignupStatus(ctx context.Context, teamID uuid.UUID, sessionID string) (*EmbeddedSignupStatusResult, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, serviceerrors.NewValidation("session_id", "invalid session id")
	}
	return s.getEmbeddedSignupStatus(ctx, teamID, id)
}

func (s *EmbeddedSignupService) GetEmbeddedSignupStatus(ctx context.Context, teamID, channelID uuid.UUID, sessionID string) (*EmbeddedSignupStatusResult, error) {
	session, err := s.loadOwnedSession(ctx, teamID, channelID, sessionID)
	if err != nil {
		return nil, err
	}
	return s.sessionToStatus(ctx, teamID, channelID, session)
}

func (s *EmbeddedSignupService) getEmbeddedSignupStatus(ctx context.Context, teamID, sessionID uuid.UUID) (*EmbeddedSignupStatusResult, error) {
	if s.sessions == nil {
		return nil, serviceerrors.NewValidation("meta", "embedded signup sessions are not configured")
	}
	session, err := s.sessions.FindByID(ctx, teamID, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, serviceerrors.ErrNotFound
		}
		return nil, serviceerrors.MapRepository(err)
	}
	s.expireIfNeeded(ctx, session)
	channelID := uuid.Nil
	if session.ChannelID != nil {
		channelID = *session.ChannelID
	}
	return s.sessionToStatus(ctx, teamID, channelID, session)
}

func (s *EmbeddedSignupService) sessionToStatus(ctx context.Context, teamID, channelID uuid.UUID, session *EmbeddedSignupSession) (*EmbeddedSignupStatusResult, error) {
	businessID := ""
	if session.BusinessID != nil {
		businessID = *session.BusinessID
	}
	wabaID := ""
	phoneID := ""
	if session.Status == EmbeddedSignupCompleted && channelID != uuid.Nil {
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

// CompleteTeamEmbeddedSignup validates Meta assets, subscribes the app, then persists channel data.
func (s *EmbeddedSignupService) CompleteTeamEmbeddedSignup(
	ctx context.Context,
	teamID, userID uuid.UUID,
	startToken string,
	input CompleteEmbeddedSignupInput,
) (*CompleteEmbeddedSignupResult, error) {
	if s.cfg.AppSecret == "" {
		return nil, serviceerrors.NewValidation("meta", "embedded signup is not configured")
	}
	claims, err := parseSignupStartToken(s.cfg.AppSecret, startToken)
	if err != nil {
		return nil, serviceerrors.NewValidation("session_id", "invalid or expired signup session")
	}
	if claims.TeamID != teamID || claims.UserID != userID {
		return nil, serviceerrors.ErrNotFound
	}
	if state := strings.TrimSpace(input.OAuthState); state == "" || state != claims.OAuthState {
		return nil, serviceerrors.NewValidation("oauth_state", "invalid oauth state")
	}

	return s.finalizeEmbeddedSignup(ctx, teamID, userID, nil, "", claims.OAuthState, input)
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

	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	var userID uuid.UUID
	if session.UserID != nil {
		userID = *session.UserID
	}
	return s.finalizeEmbeddedSignup(ctx, teamID, userID, ch, sessionID, session.OAuthState, input)
}

func (s *EmbeddedSignupService) finalizeEmbeddedSignup(
	ctx context.Context,
	teamID, userID uuid.UUID,
	existing *channel.Channel,
	pendingSessionID, expectedOAuthState string,
	input CompleteEmbeddedSignupInput,
) (*CompleteEmbeddedSignupResult, error) {
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
	if wabaID == strings.TrimSpace(s.cfg.AppID) {
		return nil, serviceerrors.NewValidation("waba_id", "invalid WhatsApp Business Account ID")
	}

	codeHash := hashOAuthCode(code)
	if s.sessions != nil {
		if used, err := s.sessions.FindByOAuthCodeHash(ctx, teamID, codeHash); err == nil && used != nil {
			if pendingSessionID == "" || used.ID.String() != pendingSessionID {
				return nil, serviceerrors.NewValidation("code", "authorization code has already been used")
			}
		}
	}

	tokenResp, err := s.meta.ExchangeEmbeddedCode(ctx, s.cfg.AppID, s.cfg.AppSecret, code)
	if err != nil {
		return nil, mapOAuthError(err)
	}

	display, err := s.validateEmbeddedAssets(ctx, wabaID, phoneNumberID, businessID, tokenResp.AccessToken)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var ch *channel.Channel
	created := false
	if existing != nil {
		ch = existing
	} else {
		connType := channel.ConnectionEmbeddedCoex
		ch = &channel.Channel{
			ID:             uuid.New(),
			TeamID:         teamID,
			Name:           "WhatsApp Business",
			Provider:       "meta_whatsapp",
			Status:         channel.StatusConnected,
			ConnectionType: &connType,
			IsCoex:         true,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.channels.Create(ctx, ch); err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
		created = true
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
		if created {
			_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
		}
		return nil, serviceerrors.MapRepository(err)
	}

	if s.cipher == nil {
		if created {
			_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
		}
		return nil, errors.New("token encryption is not configured")
	}
	enc, err := s.cipher.Encrypt(tokenResp.AccessToken)
	if err != nil {
		if created {
			_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
		}
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
		if created {
			_ = s.channels.DeleteCredentials(ctx, ch.ID)
			_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
		}
		return nil, serviceerrors.MapRepository(err)
	}

	ob := &channel.Onboarding{ChannelID: ch.ID, Status: channel.OnboardingCompleted, CompletedAt: &now, UpdatedAt: now}
	if err := s.channels.UpsertOnboarding(ctx, ob); err != nil {
		if created {
			_ = s.channels.DeleteCredentials(ctx, ch.ID)
			_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
		}
		return nil, serviceerrors.MapRepository(err)
	}

	if s.sessions != nil {
		if pendingSessionID != "" {
			session, err := s.loadOwnedSession(ctx, teamID, ch.ID, pendingSessionID)
			if err == nil {
				session.Status = EmbeddedSignupCompleted
				session.OAuthCodeHash = &codeHash
				session.CompletedAt = &now
				if businessID != "" {
					session.BusinessID = &businessID
				}
				if err := s.sessions.Update(ctx, session); err != nil {
					if created {
						_ = s.channels.DeleteCredentials(ctx, ch.ID)
						_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
					}
					return nil, serviceerrors.MapRepository(err)
				}
			}
		} else {
			channelID := ch.ID
			var uid *uuid.UUID
			if userID != uuid.Nil {
				uid = &userID
			}
			completedSession := &EmbeddedSignupSession{
				ID:            uuid.New(),
				TeamID:        teamID,
				UserID:        uid,
				ChannelID:     &channelID,
				Status:        EmbeddedSignupCompleted,
				OAuthState:    expectedOAuthState,
				OAuthCodeHash: &codeHash,
				CreatedAt:     now,
				ExpiresAt:     now.Add(defaultSessionTTL),
				CompletedAt:   &now,
			}
			if businessID != "" {
				completedSession.BusinessID = &businessID
			}
			if err := s.sessions.Create(ctx, completedSession); err != nil {
				if created {
					_ = s.channels.DeleteCredentials(ctx, ch.ID)
					_ = s.channels.SoftDelete(ctx, teamID, ch.ID)
				}
				return nil, serviceerrors.MapRepository(err)
			}
		}
	}

	s.ensureAppSubscribed(ctx, teamID, ch.ID, wabaID, tokenResp.AccessToken)

	if s.permissionRefresher != nil {
		if err := s.permissionRefresher.RefreshPermissionsAfterConnect(ctx, teamID, ch.ID, tokenResp.AccessToken); err != nil {
			s.log.Warn("embedded_signup_permission_refresh_failed",
				"event", "embedded_signup_permission_refresh_failed",
				"team_id", teamID.String(),
				"channel_id", ch.ID.String(),
				"error", err.Error(),
			)
		}
	}

	if s.readiness != nil {
		if err := s.readiness.RefreshReadinessInternal(ctx, teamID); err != nil {
			s.log.Warn("embedded_signup_readiness_refresh_failed",
				"event", "embedded_signup_readiness_refresh_failed",
				"team_id", teamID.String(),
				"channel_id", ch.ID.String(),
				"error", err.Error(),
			)
		}
	}

	s.log.Info("embedded_signup_completed",
		"event", "embedded_signup_completed",
		"team_id", teamID.String(),
		"channel_id", ch.ID.String(),
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

func (s *EmbeddedSignupService) ensureAppSubscribed(ctx context.Context, teamID, channelID uuid.UUID, wabaID, accessToken string) {
	if strings.TrimSpace(wabaID) == "" || strings.TrimSpace(accessToken) == "" {
		return
	}
	if err := s.meta.SubscribeAppToWABA(ctx, wabaID, accessToken); err != nil && !metapkg.IsSubscribeAlreadyDone(err) {
		s.log.Warn("embedded_signup_subscribe_failed",
			"event", "embedded_signup_subscribe_failed",
			"team_id", teamID.String(),
			"channel_id", channelID.String(),
			"waba_id", wabaID,
			"error", err.Error(),
		)
	}
}

func (s *EmbeddedSignupService) validateEmbeddedAssets(
	ctx context.Context,
	wabaID, phoneNumberID, businessID, accessToken string,
) (string, error) {
	phone, err := s.meta.GetPhoneNumber(ctx, phoneNumberID, accessToken)
	if err != nil {
		return "", serviceerrors.NewValidation("phone_number_id", "invalid phone number")
	}
	numbers, err := s.meta.GetPhoneNumbers(ctx, wabaID, accessToken)
	if err != nil {
		return "", serviceerrors.NewValidation("waba_id", "invalid WhatsApp Business Account ID")
	}
	found := false
	for _, n := range numbers {
		if n.ID == phoneNumberID {
			found = true
			break
		}
	}
	if !found {
		return "", serviceerrors.NewValidation("phone_number_id", "phone number does not belong to the WABA")
	}
	if businessID != "" {
		if _, err := s.meta.GetBusiness(ctx, businessID, accessToken); err != nil {
			return "", serviceerrors.NewValidation("business_id", "invalid business id")
		}
	}
	display := phoneNumberID
	if phone.DisplayPhoneNumber != "" {
		display = phone.DisplayPhoneNumber
	}
	return display, nil
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
	if session.ChannelID == nil || *session.ChannelID != channelID {
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
