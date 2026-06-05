package broadcast

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var (
	ErrForbidden           = errors.New("insufficient permissions")
	ErrNameRequired        = errors.New("campaign name is required")
	ErrMessageRequired     = errors.New("message is required")
	ErrRecipientsRequired  = errors.New("at least one recipient is required")
	ErrDeviceNotConnected  = errors.New("device is not connected")
	ErrDeviceNotFound      = errors.New("device not found")
	ErrCannotCancel        = errors.New("broadcast cannot be cancelled")
)

type Service struct {
	broadcasts Repository
	contacts   contact.Repository
	devices    device.Repository
	teams      team.Repository
	meta       *metaclient.Client
}

func NewService(
	broadcasts Repository,
	contacts contact.Repository,
	devices device.Repository,
	teams team.Repository,
	meta *metaclient.Client,
) *Service {
	return &Service{
		broadcasts: broadcasts,
		contacts:   contacts,
		devices:    devices,
		teams:      teams,
		meta:       meta,
	}
}

type CreateInput struct {
	Name        string
	Message     string
	DeviceID    uuid.UUID
	ContactIDs  []uuid.UUID
	TagIDs      []uuid.UUID
	ScheduledAt *time.Time
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID) ([]Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	items, err := s.broadcasts.ListByTeam(ctx, teamID, 100)
	if err != nil {
		return nil, err
	}
	responses := make([]Response, 0, len(items))
	for i := range items {
		responses = append(responses, ToResponse(&items[i]))
	}
	return responses, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID, broadcastID uuid.UUID) (*Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.broadcasts.FindByID(ctx, teamID, broadcastID)
	if err != nil {
		return nil, err
	}
	resp := ToResponse(item)
	return &resp, nil
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, role team.Role, input CreateInput) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	message := strings.TrimSpace(input.Message)
	if name == "" {
		return nil, ErrNameRequired
	}
	if message == "" {
		return nil, ErrMessageRequired
	}

	dev, err := s.devices.FindByID(ctx, teamID, input.DeviceID)
	if err != nil {
		return nil, ErrDeviceNotFound
	}
	if dev.Status != device.StatusConnected || dev.AccessToken == nil || dev.PhoneNumberID == nil {
		return nil, ErrDeviceNotConnected
	}

	recipients, err := s.resolveRecipients(ctx, teamID, input.ContactIDs, input.TagIDs)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, ErrRecipientsRequired
	}

	now := time.Now().UTC()
	status := StatusSending
	var scheduledAt *time.Time
	if input.ScheduledAt != nil {
		at := input.ScheduledAt.UTC()
		scheduledAt = &at
		if at.After(now) {
			status = StatusScheduled
		}
	}

	broadcast := &Broadcast{
		ID:              uuid.New(),
		TeamID:          teamID,
		DeviceID:        input.DeviceID,
		Name:            name,
		Message:         message,
		Status:          status,
		ScheduledAt:     scheduledAt,
		CreatedBy:       userID,
		TotalRecipients: len(recipients),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.broadcasts.Create(ctx, broadcast); err != nil {
		return nil, err
	}
	if err := s.broadcasts.AddBroadcastTags(ctx, broadcast.ID, input.TagIDs); err != nil {
		return nil, err
	}

	recipientRows := make([]Recipient, 0, len(recipients))
	for _, rc := range recipients {
		contactID := rc.ID
		recipientRows = append(recipientRows, Recipient{
			ID:          uuid.New(),
			BroadcastID: broadcast.ID,
			ContactID:   &contactID,
			Phone:       rc.Phone,
			Status:      "pending",
			CreatedAt:   now,
		})
	}
	if err := s.broadcasts.AddRecipients(ctx, recipientRows); err != nil {
		return nil, err
	}

	if status == StatusSending {
		go s.dispatchBroadcast(context.Background(), broadcast.ID, teamID)
	}

	resp := ToResponse(broadcast)
	return &resp, nil
}

func (s *Service) Cancel(ctx context.Context, teamID, userID uuid.UUID, role team.Role, broadcastID uuid.UUID) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	broadcast, err := s.broadcasts.FindByID(ctx, teamID, broadcastID)
	if err != nil {
		return nil, err
	}
	if broadcast.Status != StatusScheduled && broadcast.Status != StatusDraft {
		return nil, ErrCannotCancel
	}

	broadcast.Status = StatusCancelled
	broadcast.UpdatedAt = time.Now().UTC()
	if err := s.broadcasts.Update(ctx, broadcast); err != nil {
		return nil, err
	}
	resp := ToResponse(broadcast)
	return &resp, nil
}

func (s *Service) ProcessDue(ctx context.Context) error {
	now := time.Now().UTC()
	due, err := s.broadcasts.ListDueScheduled(ctx, now, 20)
	if err != nil {
		return err
	}
	for i := range due {
		item := due[i]
		item.Status = StatusSending
		item.UpdatedAt = now
		if err := s.broadcasts.Update(ctx, &item); err != nil {
			continue
		}
		go s.dispatchBroadcast(context.Background(), item.ID, item.TeamID)
	}
	return nil
}

func (s *Service) dispatchBroadcast(ctx context.Context, broadcastID, teamID uuid.UUID) {
	broadcast, err := s.broadcasts.FindByID(ctx, teamID, broadcastID)
	if err != nil {
		return
	}

	dev, err := s.devices.FindByID(ctx, teamID, broadcast.DeviceID)
	if err != nil || dev.AccessToken == nil || dev.PhoneNumberID == nil {
		broadcast.Status = StatusFailed
		broadcast.UpdatedAt = time.Now().UTC()
		_ = s.broadcasts.Update(ctx, broadcast)
		return
	}

	now := time.Now().UTC()
	if broadcast.StartedAt == nil {
		broadcast.StartedAt = &now
	}

	pending, err := s.broadcasts.ListPendingRecipients(ctx, broadcastID)
	if err != nil {
		return
	}

	sent := broadcast.SentCount
	failed := broadcast.FailedCount

	for i := range pending {
		recipient := pending[i]
		wamid, sendErr := s.meta.SendTextMessage(ctx, *dev.PhoneNumberID, *dev.AccessToken, recipient.Phone, broadcast.Message)
		sentAt := time.Now().UTC()
		if sendErr != nil {
			msg := sendErr.Error()
			recipient.Status = "failed"
			recipient.ErrorMessage = &msg
			failed++
		} else {
			recipient.Status = "sent"
			recipient.WAMID = &wamid
			recipient.SentAt = &sentAt
			sent++
		}
		_ = s.broadcasts.UpdateRecipient(ctx, &recipient)
		time.Sleep(200 * time.Millisecond)
	}

	broadcast.SentCount = sent
	broadcast.FailedCount = failed
	broadcast.UpdatedAt = time.Now().UTC()
	completedAt := time.Now().UTC()
	broadcast.CompletedAt = &completedAt
	if failed > 0 && sent == 0 {
		broadcast.Status = StatusFailed
	} else {
		broadcast.Status = StatusCompleted
	}
	_ = s.broadcasts.Update(ctx, broadcast)
}

func (s *Service) resolveRecipients(ctx context.Context, teamID uuid.UUID, contactIDs, tagIDs []uuid.UUID) ([]contact.Contact, error) {
	unique := make(map[string]contact.Contact)

	if len(contactIDs) > 0 {
		items, err := s.contacts.ListByIDs(ctx, teamID, contactIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			unique[item.Phone] = item
		}
	}

	if len(tagIDs) > 0 {
		items, err := s.contacts.ListByTagIDs(ctx, teamID, tagIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			unique[item.Phone] = item
		}
	}

	result := make([]contact.Contact, 0, len(unique))
	for _, item := range unique {
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}
