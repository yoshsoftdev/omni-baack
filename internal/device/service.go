package device

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
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
}

func NewService(devices Repository, teams team.Repository, meta *metaclient.Client) *Service {
	return &Service{devices: devices, teams: teams, meta: meta}
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
		ID:        uuid.New(),
		TeamID:    teamID,
		Name:      name,
		Provider:  "meta",
		Status:    StatusDisconnected,
		CreatedAt: now,
		UpdatedAt: now,
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
