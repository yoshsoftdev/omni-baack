package channel

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (ChannelService) manages WhatsApp channels for a workspace.
type Service struct {
	channels Repository
	teams    team.Repository
	quota    *quota.Service
	rbac     *rbac.Service
	cipher   TokenCipher
	meta     ManualConnectMetaClient
}

// TokenCipher encrypts Meta access tokens for storage.
type TokenCipher interface {
	Encrypt(plaintext string) ([]byte, error)
}

func NewService(
	channels Repository,
	teams team.Repository,
	quotaSvc *quota.Service,
	rbacSvc *rbac.Service,
	cipher TokenCipher,
	meta ManualConnectMetaClient,
) *Service {
	return &Service{
		channels: channels,
		teams:    teams,
		quota:    quotaSvc,
		rbac:     rbacSvc,
		cipher:   cipher,
		meta:     meta,
	}
}

type ManualConnectInput struct {
	AccessToken   string
	WABAID        string
	PhoneNumberID string
	BusinessID    *string
}

type CreateInput struct {
	Name           string
	Provider       string
	ConnectionType *ConnectionType
	PhoneNumber    *string
	BusinessID     *string
	Manual         *ManualConnectInput
}

type UpdateInput struct {
	Name        *string
	PhoneNumber *string
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Channel, error) {
	if err := s.authorize(actorRole, rbac.PermDeviceManage); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := s.quota.AssertChannelHeadroom(ctx, teamID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, serviceerrors.NewValidation("name", "is required")
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "meta_whatsapp"
	}

	if input.Manual != nil {
		if _, err := s.verifyManualConnect(ctx, *input.Manual); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	ch := &Channel{
		ID:             uuid.New(),
		TeamID:         teamID,
		Name:           name,
		Provider:       provider,
		Status:         StatusDisconnected,
		ConnectionType: input.ConnectionType,
		PhoneNumber:    input.PhoneNumber,
		BusinessID:     input.BusinessID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := serviceerrors.MapRepository(s.channels.Create(ctx, ch)); err != nil {
		return nil, err
	}

	ob := &Onboarding{
		ChannelID: ch.ID,
		Status:    OnboardingPending,
		UpdatedAt: now,
	}
	if err := serviceerrors.MapRepository(s.channels.UpsertOnboarding(ctx, ob)); err != nil {
		return nil, err
	}

	if input.Manual != nil {
		if err := s.applyManualConnect(ctx, ch, input.Manual); err != nil {
			return nil, err
		}
		updated, err := s.channels.FindByID(ctx, teamID, ch.ID)
		if err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
		return updated, nil
	}
	return ch, nil
}

func (s *Service) applyManualConnect(ctx context.Context, ch *Channel, manual *ManualConnectInput) error {
	token := strings.TrimSpace(manual.AccessToken)
	wabaID := strings.TrimSpace(manual.WABAID)
	phoneNumberID := strings.TrimSpace(manual.PhoneNumberID)
	if token == "" || wabaID == "" || phoneNumberID == "" {
		return serviceerrors.NewValidation("manual_connect", "access_token, waba_id, and phone_number_id are required")
	}
	if s.cipher == nil {
		return serviceerrors.NewValidation("manual_connect", "token encryption is not configured")
	}

	now := time.Now().UTC()
	connType := ConnectionManual
	ch.Status = StatusConnected
	ch.ConnectionType = &connType
	ch.WABAID = &wabaID
	ch.PhoneNumberID = &phoneNumberID
	if manual.BusinessID != nil && strings.TrimSpace(*manual.BusinessID) != "" {
		businessID := strings.TrimSpace(*manual.BusinessID)
		ch.BusinessID = &businessID
	}
	ch.IsCoex = false
	ch.LastConnectedAt = &now
	ch.UpdatedAt = now
	if err := s.channels.Update(ctx, ch); err != nil {
		return serviceerrors.MapRepository(err)
	}

	enc, err := s.cipher.Encrypt(token)
	if err != nil {
		return err
	}
	if err := s.channels.UpsertCredentials(ctx, &Credentials{
		ChannelID:      ch.ID,
		AccessTokenEnc: enc,
		UpdatedAt:      now,
	}); err != nil {
		return serviceerrors.MapRepository(err)
	}

	ob := &Onboarding{
		ChannelID:   ch.ID,
		Status:      OnboardingCompleted,
		CompletedAt: &now,
		UpdatedAt:   now,
	}
	return serviceerrors.MapRepository(s.channels.UpsertOnboarding(ctx, ob))
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*Channel, error) {
	if err := s.authorize(actorRole, rbac.PermDeviceView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return ch, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Channel], error) {
	if err := s.authorize(actorRole, rbac.PermDeviceView); err != nil {
		return repository.Page[Channel]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Channel]{}, err
	}
	result, err := s.channels.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Channel]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID, input UpdateInput) (*Channel, error) {
	if err := s.authorize(actorRole, rbac.PermDeviceManage); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		ch.Name = name
	}
	if input.PhoneNumber != nil {
		ch.PhoneNumber = input.PhoneNumber
	}
	ch.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.channels.Update(ctx, ch)); err != nil {
		return nil, err
	}
	return ch, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermDeviceManage); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.channels.SoftDelete(ctx, teamID, channelID))
}

func (s *Service) GetOnboarding(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*Onboarding, error) {
	if err := s.authorize(actorRole, rbac.PermDeviceView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.channels.FindByID(ctx, teamID, channelID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	ob, err := s.channels.FindOnboarding(ctx, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return ob, nil
}

// ValidateForCampaign ensures the channel is owned by the team and ready for broadcast use.
func (s *Service) ValidateForCampaign(ctx context.Context, teamID, channelID uuid.UUID) error {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if ch.Status != StatusConnected {
		return serviceerrors.NewValidation("channel_id", "channel must be connected")
	}
	ob, err := s.channels.FindOnboarding(ctx, channelID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if ob.Status != OnboardingCompleted {
		return serviceerrors.NewValidation("channel_id", "channel onboarding must be completed")
	}
	return nil
}
