package device

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var (
	ErrForbidden        = errors.New("insufficient permissions")
	ErrNameRequired     = errors.New("device name is required")
	ErrAlreadyConnected = errors.New("device is already connected")
	ErrMetaRequired     = errors.New("phone_number_id, waba_id, and access_token are required")
	ErrMetaInvalid      = errors.New("invalid meta credentials")
)

type Service struct {
	devices Repository
	teams   team.Repository
	meta    *metaclient.Client
	metaCfg config.MetaConfig
}

func NewService(devices Repository, teams team.Repository, meta *metaclient.Client, metaCfg config.MetaConfig) *Service {
	return &Service{devices: devices, teams: teams, meta: meta, metaCfg: metaCfg}
}

type CreateInput struct {
	Name string
}

type UpdateInput struct {
	Name string
}

type ConnectInput struct {
	PhoneNumberID string
	WabaID        string
	AccessToken   string
}

type ConnectEmbeddedInput struct {
	Code          string
	WabaID        string
	PhoneNumberID string
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID) ([]Device, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.devices.ListByTeam(ctx, teamID)
}

func (s *Service) Get(ctx context.Context, teamID, userID, deviceID uuid.UUID) (*Device, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.devices.FindByID(ctx, teamID, deviceID)
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, role team.Role, input CreateInput) (*Device, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrNameRequired
	}

	now := time.Now().UTC()
	device := &Device{
		ID:             uuid.New(),
		TeamID:         teamID,
		Name:           name,
		Provider:       "meta",
		Status:         StatusDisconnected,
		ConnectionType: ConnectionManual,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.devices.Create(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, role team.Role, deviceID uuid.UUID, input UpdateInput) (*Device, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrNameRequired
	}

	device, err := s.devices.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}

	device.Name = name
	device.UpdatedAt = time.Now().UTC()
	if err := s.devices.Update(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, role team.Role, deviceID uuid.UUID) error {
	if !canManage(role) {
		return ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return s.devices.SoftDelete(ctx, teamID, deviceID)
}

// Connect validates Meta WhatsApp Cloud API credentials and links the device.
func (s *Service) Connect(ctx context.Context, teamID, userID uuid.UUID, role team.Role, deviceID uuid.UUID, input ConnectInput) (*Device, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	phoneNumberID := strings.TrimSpace(input.PhoneNumberID)
	wabaID := strings.TrimSpace(input.WabaID)
	accessToken := strings.TrimSpace(input.AccessToken)
	if phoneNumberID == "" || wabaID == "" || accessToken == "" {
		return nil, ErrMetaRequired
	}

	device, err := s.devices.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}
	if device.Status == StatusConnected {
		return nil, ErrAlreadyConnected
	}

	device.Status = StatusConnecting
	device.UpdatedAt = time.Now().UTC()
	if err := s.devices.Update(ctx, device); err != nil {
		return nil, err
	}

	phone, err := s.meta.GetPhoneNumber(ctx, phoneNumberID, accessToken)
	if err != nil {
		device.Status = StatusDisconnected
		_ = s.devices.Update(ctx, device)
		return nil, ErrMetaInvalid
	}

	now := time.Now().UTC()
	display := phone.DisplayPhoneNumber
	device.Status = StatusConnected
	device.WabaID = &wabaID
	device.PhoneNumberID = &phoneNumberID
	device.AccessToken = &accessToken
	device.PhoneNumber = &display
	device.LastConnectedAt = &now
	device.UpdatedAt = now

	if err := s.devices.Update(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

// ConnectEmbedded completes Meta Embedded Signup (coexistence) and links the device.
func (s *Service) ConnectEmbedded(ctx context.Context, teamID, userID uuid.UUID, role team.Role, deviceID uuid.UUID, input ConnectEmbeddedInput) (*Device, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.metaCfg.AppID == "" || s.metaCfg.AppSecret == "" {
		return nil, errors.New("meta embedded signup is not configured")
	}

	code := strings.TrimSpace(input.Code)
	wabaID := strings.TrimSpace(input.WabaID)
	phoneNumberID := strings.TrimSpace(input.PhoneNumberID)
	if code == "" || wabaID == "" {
		return nil, errors.New("code and waba_id are required")
	}

	device, err := s.devices.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}
	if device.Status == StatusConnected {
		return nil, ErrAlreadyConnected
	}

	device.Status = StatusConnecting
	device.UpdatedAt = time.Now().UTC()
	if err := s.devices.Update(ctx, device); err != nil {
		return nil, err
	}

	token, err := s.meta.ExchangeEmbeddedCode(ctx, s.metaCfg.AppID, s.metaCfg.AppSecret, code)
	if err != nil {
		device.Status = StatusDisconnected
		_ = s.devices.Update(ctx, device)
		return nil, ErrMetaInvalid
	}

	if phoneNumberID == "" {
		phones, listErr := s.meta.ListWABAPhoneNumbers(ctx, wabaID, token.AccessToken)
		if listErr != nil || len(phones) == 0 {
			device.Status = StatusDisconnected
			_ = s.devices.Update(ctx, device)
			return nil, ErrMetaInvalid
		}
		phoneNumberID = phones[0].ID
	}

	phone, err := s.meta.GetPhoneNumber(ctx, phoneNumberID, token.AccessToken)
	if err != nil {
		device.Status = StatusDisconnected
		_ = s.devices.Update(ctx, device)
		return nil, ErrMetaInvalid
	}

	now := time.Now().UTC()
	display := phone.DisplayPhoneNumber
	pending := OnboardingPending
	device.Status = StatusConnected
	device.ConnectionType = ConnectionEmbedded
	device.IsCoex = true
	device.OnboardingStatus = &pending
	device.WabaID = &wabaID
	device.PhoneNumberID = &phoneNumberID
	device.AccessToken = &token.AccessToken
	device.PhoneNumber = &display
	device.LastConnectedAt = &now
	device.UpdatedAt = now
	device.OnboardingError = nil
	device.OnboardingCompletedAt = nil
	device.ContactsSyncedCount = 0
	device.HistoryMessagesSynced = 0
	device.HistorySyncProgress = 0

	if err := s.devices.Update(ctx, device); err != nil {
		return nil, err
	}

	_ = s.meta.SubscribeAppToWABA(ctx, wabaID, token.AccessToken)

	return device, nil
}

func (s *Service) Disconnect(ctx context.Context, teamID, userID uuid.UUID, role team.Role, deviceID uuid.UUID) (*Device, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	device, err := s.devices.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}

	device.Status = StatusDisconnected
	device.WabaID = nil
	device.PhoneNumberID = nil
	device.AccessToken = nil
	device.PhoneNumber = nil
	device.ConnectionType = ConnectionManual
	device.IsCoex = false
	device.OnboardingStatus = nil
	device.OnboardingError = nil
	device.OnboardingCompletedAt = nil
	device.ContactsSyncedCount = 0
	device.HistoryMessagesSynced = 0
	device.HistorySyncProgress = 0
	device.ContactsSyncRequestID = nil
	device.HistorySyncRequestID = nil
	device.UpdatedAt = time.Now().UTC()

	if err := s.devices.Update(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}
