package onboarding

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var (
	ErrForbidden          = errors.New("insufficient permissions")
	ErrDeviceNotConnected = errors.New("device is not connected")
	ErrNotCoexDevice      = errors.New("device is not a coexistence device")
	ErrSyncAlreadyStarted = errors.New("onboarding sync already started")
)

type Service struct {
	devices device.Repository
	teams   team.Repository
	meta    *metaclient.Client
}

func NewService(
	devices device.Repository,
	teams team.Repository,
	meta *metaclient.Client,
) *Service {
	return &Service{
		devices: devices,
		teams:   teams,
		meta:    meta,
	}
}

type StatusResponse struct {
	Status                *device.OnboardingStatus `json:"status"`
	ContactsSyncedCount   int                      `json:"contacts_synced_count"`
	HistoryMessagesSynced int                      `json:"history_messages_synced"`
	HistorySyncProgress   int                      `json:"history_sync_progress"`
	Error                 *string                  `json:"error,omitempty"`
	CompletedAt           *time.Time               `json:"completed_at,omitempty"`
}

func (s *Service) GetStatus(ctx context.Context, teamID, userID, deviceID uuid.UUID) (*StatusResponse, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	dev, err := s.devices.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}
	return toStatusResponse(dev), nil
}

func (s *Service) StartSync(ctx context.Context, teamID, userID uuid.UUID, role team.Role, deviceID uuid.UUID) (*StatusResponse, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	dev, err := s.devices.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}
	if dev.Status != device.StatusConnected || dev.AccessToken == nil || dev.PhoneNumberID == nil {
		return nil, ErrDeviceNotConnected
	}
	if !dev.IsCoex {
		return nil, ErrNotCoexDevice
	}
	if dev.ContactsSyncRequestID != nil || dev.HistorySyncRequestID != nil {
		if dev.OnboardingStatus != nil && *dev.OnboardingStatus == device.OnboardingCompleted {
			return toStatusResponse(dev), nil
		}
		return nil, ErrSyncAlreadyStarted
	}

	syncingContacts := device.OnboardingSyncingContacts
	dev.OnboardingStatus = &syncingContacts
	dev.OnboardingError = nil
	dev.UpdatedAt = time.Now().UTC()

	contactResp, err := s.meta.RequestSMBSync(ctx, *dev.PhoneNumberID, *dev.AccessToken, "smb_app_state_sync")
	if err != nil {
		errMsg := err.Error()
		failed := device.OnboardingFailed
		dev.OnboardingStatus = &failed
		dev.OnboardingError = &errMsg
		_ = s.devices.Update(ctx, dev)
		return nil, err
	}
	dev.ContactsSyncRequestID = &contactResp.RequestID

	historyResp, err := s.meta.RequestSMBSync(ctx, *dev.PhoneNumberID, *dev.AccessToken, "history")
	if err != nil {
		errMsg := err.Error()
		failed := device.OnboardingFailed
		dev.OnboardingStatus = &failed
		dev.OnboardingError = &errMsg
		_ = s.devices.Update(ctx, dev)
		return nil, err
	}
	dev.HistorySyncRequestID = &historyResp.RequestID
	syncingHistory := device.OnboardingSyncingHistory
	dev.OnboardingStatus = &syncingHistory

	if err := s.devices.Update(ctx, dev); err != nil {
		return nil, err
	}
	return toStatusResponse(dev), nil
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}

func toStatusResponse(dev *device.Device) *StatusResponse {
	return &StatusResponse{
		Status:                dev.OnboardingStatus,
		ContactsSyncedCount:   dev.ContactsSyncedCount,
		HistoryMessagesSynced: dev.HistoryMessagesSynced,
		HistorySyncProgress:   dev.HistorySyncProgress,
		Error:                 dev.OnboardingError,
		CompletedAt:           dev.OnboardingCompletedAt,
	}
}
