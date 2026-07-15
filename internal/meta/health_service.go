package meta

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const templateSyncFreshness = 24 * time.Hour

type HealthChecks struct {
	Token        bool `json:"token"`
	WABA         bool `json:"waba,omitempty"`
	Phone        bool `json:"phone,omitempty"`
	TemplatesAPI bool `json:"templates_api,omitempty"`
	TemplateSync bool `json:"template_sync,omitempty"`
	Profile      bool `json:"profile,omitempty"`
}

type HealthResult struct {
	Healthy bool         `json:"healthy"`
	Checks  HealthChecks `json:"checks"`
}

// TemplateSyncChecker reports whether a channel has a recent successful template sync.
type TemplateSyncChecker interface {
	ChannelSyncFresh(ctx context.Context, channelID uuid.UUID) bool
}

type HealthService struct {
	meta        MetaClient
	instagram   *metapkg.Client
	channels    channel.Repository
	cipher      *cryptoutil.TokenCipher
	syncChecker TemplateSyncChecker
}

func NewHealthService(meta MetaClient, instagram *metapkg.Client, channels channel.Repository, cipher *cryptoutil.TokenCipher, syncChecker TemplateSyncChecker) *HealthService {
	return &HealthService{meta: meta, instagram: instagram, channels: channels, cipher: cipher, syncChecker: syncChecker}
}

func (s *HealthService) CheckChannel(ctx context.Context, teamID, channelID uuid.UUID) (*HealthResult, error) {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	result := &HealthResult{Checks: HealthChecks{}}

	creds, err := s.channels.FindCredentials(ctx, channelID)
	if err != nil || s.cipher == nil {
		return result, nil
	}

	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil || strings.TrimSpace(token) == "" {
		return result, nil
	}
	if creds.TokenExpiresAt != nil && creds.TokenExpiresAt.Before(time.Now().UTC()) {
		return result, nil
	}

	if ch.ChannelType == channel.TypeInstagram {
		return s.checkInstagramChannel(ctx, token, result)
	}

	if ch.WABAID == nil || ch.PhoneNumberID == nil {
		return result, nil
	}

	result.Checks.Token = s.meta.Ping(ctx, token) == nil
	if !result.Checks.Token {
		return result, nil
	}

	wabaID := strings.TrimSpace(*ch.WABAID)
	phoneID := strings.TrimSpace(*ch.PhoneNumberID)

	if _, err := s.meta.GetWABA(ctx, wabaID, token); err == nil {
		result.Checks.WABA = true
	}
	if _, err := s.meta.GetPhoneNumber(ctx, phoneID, token); err == nil {
		result.Checks.Phone = true
	}
	if _, err := s.meta.ListTemplates(ctx, wabaID, token); err == nil {
		result.Checks.TemplatesAPI = true
	}
	if s.syncChecker != nil {
		result.Checks.TemplateSync = s.syncChecker.ChannelSyncFresh(ctx, channelID)
	}

	result.Healthy = result.Checks.Token &&
		result.Checks.WABA &&
		result.Checks.Phone &&
		result.Checks.TemplatesAPI &&
		result.Checks.TemplateSync
	return result, nil
}

func (s *HealthService) checkInstagramChannel(ctx context.Context, token string, result *HealthResult) (*HealthResult, error) {
	if s.instagram == nil {
		return result, nil
	}
	if _, err := s.instagram.GetInstagramUser(ctx, token); err == nil {
		result.Checks.Token = true
		result.Checks.Profile = true
	}
	result.Healthy = result.Checks.Token && result.Checks.Profile
	return result, nil
}
