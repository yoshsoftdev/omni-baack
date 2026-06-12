package onboarding

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
)

var (
	ErrForbidden          = errors.New("insufficient permissions")
	ErrDeviceNotConnected = errors.New("device is not connected")
	ErrNotCoexDevice      = errors.New("device is not a coexistence device")
	ErrSyncAlreadyStarted = errors.New("onboarding sync already started")
)

type Service struct {
	devices       device.Repository
	contacts      *contact.Service
	conversations *conversation.Service
	messages      message.Repository
	teams         team.Repository
	meta          *metaclient.Client
}

func NewService(
	devices device.Repository,
	contacts *contact.Service,
	conversations *conversation.Service,
	messages message.Repository,
	teams team.Repository,
	meta *metaclient.Client,
) *Service {
	return &Service{
		devices:       devices,
		contacts:      contacts,
		conversations: conversations,
		messages:      messages,
		teams:         teams,
		meta:          meta,
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

func (s *Service) HandleWebhook(ctx context.Context, events *metaclient.CoexWebhookEvents) error {
	for i := range events.ContactSyncEvents {
		if err := s.handleContactSync(ctx, &events.ContactSyncEvents[i]); err != nil {
			return err
		}
	}
	for i := range events.HistoryEvents {
		if err := s.handleHistory(ctx, &events.HistoryEvents[i]); err != nil {
			return err
		}
	}
	for i := range events.MessageEchoes {
		if err := s.handleMessageEcho(ctx, &events.MessageEchoes[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleContactSync(ctx context.Context, event *metaclient.ContactSyncEvent) error {
	dev, err := s.devices.FindByPhoneNumberID(ctx, event.PhoneNumberID)
	if err != nil {
		return nil
	}
	if event.Action == "remove" {
		return nil
	}

	rawPhone := phone.Normalize(event.PhoneNumber)
	name := strings.TrimSpace(event.FullName)
	if name == "" {
		name = strings.TrimSpace(event.FirstName)
	}
	if name == "" {
		name = rawPhone
	}

	if _, err := s.contacts.FindOrCreateByPhone(ctx, dev.TeamID, rawPhone, name); err != nil {
		return err
	}

	dev.ContactsSyncedCount++
	if dev.OnboardingStatus != nil && *dev.OnboardingStatus == device.OnboardingPending {
		syncing := device.OnboardingSyncingContacts
		dev.OnboardingStatus = &syncing
	}
	dev.UpdatedAt = time.Now().UTC()
	return s.devices.Update(ctx, dev)
}

func (s *Service) handleHistory(ctx context.Context, event *metaclient.HistoryEvent) error {
	dev, err := s.devices.FindByPhoneNumberID(ctx, event.PhoneNumberID)
	if err != nil {
		return nil
	}

	if event.Declined {
		reason := event.DeclineReason
		if reason == "" {
			reason = "history sync declined by business"
		}
		dev.OnboardingError = &reason
		return s.completeOnboarding(ctx, dev)
	}

	businessPhone := ""
	if dev.PhoneNumber != nil {
		businessPhone = phone.Normalize(*dev.PhoneNumber)
	}

	for _, thread := range event.Threads {
		contactPhone := phone.Normalize(thread.ContactPhone)
		contactEntity, err := s.contacts.FindOrCreateByPhone(ctx, dev.TeamID, contactPhone, contactPhone)
		if err != nil {
			return err
		}
		conv, err := s.conversations.FindOrCreate(ctx, dev.TeamID, contactEntity.ID, dev.ID)
		if err != nil {
			return err
		}

		for _, histMsg := range thread.Messages {
			if histMsg.ID == "" {
				continue
			}
			existing, findErr := s.messages.FindByWAMID(ctx, histMsg.ID)
			if findErr == nil && existing != nil {
				continue
			}

			direction := message.DirectionInbound
			senderPhone := phone.Normalize(histMsg.From)
			if businessPhone != "" && phone.Normalize(histMsg.From) == businessPhone {
				direction = message.DirectionOutbound
				senderPhone = contactPhone
			}

			sentAt := metaclient.ParseUnixTimestamp(histMsg.Timestamp)
			deviceID := dev.ID
			contactID := contactEntity.ID
			wamid := histMsg.ID
			now := time.Now().UTC()

			msg := &message.Message{
				ID:             uuid.New(),
				TeamID:         dev.TeamID,
				ConversationID: conv.ID,
				DeviceID:       &deviceID,
				ContactID:      &contactID,
				WAMID:          &wamid,
				Direction:      direction,
				Content:        histMsg.Body,
				SenderPhone:    senderPhone,
				SenderName:     contactEntity.Name,
				SentAt:         sentAt,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := s.messages.Create(ctx, msg); err != nil {
				return err
			}
			if err := s.conversations.TouchFromMessage(ctx, conv, histMsg.Body, direction == message.DirectionInbound); err != nil {
				return err
			}
			dev.HistoryMessagesSynced++
		}
	}

	if event.Progress > dev.HistorySyncProgress {
		dev.HistorySyncProgress = event.Progress
	}
	syncing := device.OnboardingSyncingHistory
	dev.OnboardingStatus = &syncing
	dev.UpdatedAt = time.Now().UTC()

	if event.Progress >= 100 {
		return s.completeOnboarding(ctx, dev)
	}
	return s.devices.Update(ctx, dev)
}

func (s *Service) handleMessageEcho(ctx context.Context, event *metaclient.MessageEchoEvent) error {
	dev, err := s.devices.FindByPhoneNumberID(ctx, event.PhoneNumberID)
	if err != nil {
		return nil
	}

	contactPhone := phone.Normalize(event.To)
	contactEntity, err := s.contacts.FindOrCreateByPhone(ctx, dev.TeamID, contactPhone, contactPhone)
	if err != nil {
		return err
	}
	conv, err := s.conversations.FindOrCreate(ctx, dev.TeamID, contactEntity.ID, dev.ID)
	if err != nil {
		return err
	}

	if event.ID != "" {
		if existing, findErr := s.messages.FindByWAMID(ctx, event.ID); findErr == nil && existing != nil {
			return nil
		}
	}

	sentAt := metaclient.ParseUnixTimestamp(event.Timestamp)
	deviceID := dev.ID
	contactID := contactEntity.ID
	wamid := event.ID
	now := time.Now().UTC()
	senderPhone := phone.Normalize(event.From)

	msg := &message.Message{
		ID:             uuid.New(),
		TeamID:         dev.TeamID,
		ConversationID: conv.ID,
		DeviceID:       &deviceID,
		ContactID:      &contactID,
		WAMID:          &wamid,
		Direction:      message.DirectionOutbound,
		Content:        event.Body,
		SenderPhone:    senderPhone,
		SenderName:     dev.Name,
		SentAt:         sentAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.messages.Create(ctx, msg); err != nil {
		return err
	}
	return s.conversations.TouchFromMessage(ctx, conv, event.Body, false)
}

func (s *Service) completeOnboarding(ctx context.Context, dev *device.Device) error {
	completed := device.OnboardingCompleted
	now := time.Now().UTC()
	dev.OnboardingStatus = &completed
	dev.OnboardingCompletedAt = &now
	dev.HistorySyncProgress = 100
	dev.UpdatedAt = now
	return s.devices.Update(ctx, dev)
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
