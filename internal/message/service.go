package message

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
)

var (
	ErrInvalidContent       = errors.New("content is required")
	ErrDeviceNotConnected   = errors.New("device is not connected")
	ErrDeviceNotFound       = errors.New("device not found")
	ErrConversationNotFound = errors.New("conversation not found")
)

type Service struct {
	messages      Repository
	conversations *conversation.Service
	contacts      *contact.Service
	devices       device.Repository
	teams         team.Repository
	users         user.Repository
	meta          *metaclient.Client
	ws            *hub.Hub
}

func NewService(
	messages Repository,
	conversations *conversation.Service,
	contacts *contact.Service,
	devices device.Repository,
	teams team.Repository,
	users user.Repository,
	meta *metaclient.Client,
	wsHub *hub.Hub,
) *Service {
	return &Service{
		messages:      messages,
		conversations: conversations,
		contacts:      contacts,
		devices:       devices,
		teams:         teams,
		users:         users,
		meta:          meta,
		ws:            wsHub,
	}
}

func (s *Service) List(ctx context.Context, teamID, userID, conversationID uuid.UUID) ([]Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.conversations.Get(ctx, teamID, userID, conversationID); err != nil {
		return nil, err
	}

	messages, err := s.messages.ListByConversation(ctx, teamID, conversationID, 200)
	if err != nil {
		return nil, err
	}

	responses := make([]Response, 0, len(messages))
	for i := range messages {
		responses = append(responses, ToResponse(&messages[i]))
	}
	return responses, nil
}

type SendInput struct {
	Content string
}

func (s *Service) Send(ctx context.Context, teamID, userID, conversationID uuid.UUID, input SendInput) (*Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrInvalidContent
	}

	detail, err := s.conversations.Get(ctx, teamID, userID, conversationID)
	if err != nil {
		return nil, err
	}

	dev, err := s.devices.FindByID(ctx, teamID, detail.DeviceID)
	if err != nil {
		return nil, ErrDeviceNotFound
	}
	if dev.Status != device.StatusConnected || dev.AccessToken == nil || dev.PhoneNumberID == nil {
		return nil, ErrDeviceNotConnected
	}

	wamid, err := s.meta.SendTextMessage(ctx, *dev.PhoneNumberID, *dev.AccessToken, detail.ContactPhone, content)
	if err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindOrCreate(ctx, teamID, detail.ContactID, detail.DeviceID)
	if err != nil {
		return nil, err
	}

	sender, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	deviceID := dev.ID
	contactID := detail.ContactID
	wamidStr := wamid
	senderPhone := deref(dev.PhoneNumber, "system")
	senderName := sender.Name
	if strings.TrimSpace(senderName) == "" {
		senderName = sender.Email
	}

	msg := &Message{
		ID:             uuid.New(),
		TeamID:         teamID,
		ConversationID: conversationID,
		DeviceID:       &deviceID,
		ContactID:      &contactID,
		SentBy:         &userID,
		WAMID:          &wamidStr,
		Direction:      DirectionOutbound,
		Content:        content,
		SenderPhone:    senderPhone,
		SenderName:     senderName,
		SentAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.messages.Create(ctx, msg); err != nil {
		return nil, err
	}
	if err := s.conversations.TouchFromMessage(ctx, conv, content, false); err != nil {
		return nil, err
	}

	resp := ToResponse(msg)
	s.publishMessage(ctx, teamID, resp)
	_ = s.conversations.PublishConversation(ctx, teamID, "conversation.updated", conversationID)
	return &resp, nil
}

type RecordBroadcastTemplateInput struct {
	TeamID      uuid.UUID
	Device      *device.Device
	ContactID   *uuid.UUID
	Phone       string
	ContactName string
	WAMID       string
	Content     string
	SentAt      time.Time
}

// RecordBroadcastTemplate stores a successful campaign template send in conversation history.
func (s *Service) RecordBroadcastTemplate(ctx context.Context, input RecordBroadcastTemplateInput) error {
	if input.Device == nil || strings.TrimSpace(input.WAMID) == "" {
		return nil
	}

	wamid := strings.TrimSpace(input.WAMID)
	existing, err := s.messages.FindByWAMID(ctx, wamid)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	contactEntity, err := s.resolveBroadcastContact(ctx, input)
	if err != nil {
		return err
	}

	conv, err := s.conversations.FindOrCreate(ctx, input.TeamID, contactEntity.ID, input.Device.ID)
	if err != nil {
		return err
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = "[Template message]"
	}

	now := input.SentAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	deviceID := input.Device.ID
	contactID := contactEntity.ID
	senderPhone := deref(input.Device.PhoneNumber, "system")
	senderName := strings.TrimSpace(input.Device.Name)
	if senderName == "" {
		senderName = "Broadcast"
	}

	msg := &Message{
		ID:             uuid.New(),
		TeamID:         input.TeamID,
		ConversationID: conv.ID,
		DeviceID:       &deviceID,
		ContactID:      &contactID,
		WAMID:          &wamid,
		Direction:      DirectionOutbound,
		Content:        content,
		SenderPhone:    senderPhone,
		SenderName:     senderName,
		SentAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.messages.Create(ctx, msg); err != nil {
		return err
	}
	if err := s.conversations.TouchFromMessage(ctx, conv, content, false); err != nil {
		return err
	}

	resp := ToResponse(msg)
	s.publishMessage(ctx, input.TeamID, resp)
	return s.conversations.PublishConversation(ctx, input.TeamID, "conversation.updated", conv.ID)
}

func (s *Service) resolveBroadcastContact(ctx context.Context, input RecordBroadcastTemplateInput) (*contact.Contact, error) {
	displayName := strings.TrimSpace(input.ContactName)
	if displayName == "" {
		displayName = input.Phone
	}
	return s.contacts.FindOrCreateByPhone(ctx, input.TeamID, input.Phone, displayName)
}

func (s *Service) HandleMetaInbound(ctx context.Context, inbound metaclient.InboundMessage) error {
	dev, err := s.devices.FindByPhoneNumberID(ctx, inbound.PhoneNumberID)
	if err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return fmt.Errorf("device not found for phone_number_id %s: connect device in /devices first", inbound.PhoneNumberID)
		}
		return err
	}

	normalizedPhone := phone.Normalize(inbound.From)
	contactEntity, err := s.contacts.FindOrCreateByPhone(ctx, dev.TeamID, normalizedPhone, normalizedPhone)
	if err != nil {
		return err
	}

	conv, err := s.conversations.FindOrCreate(ctx, dev.TeamID, contactEntity.ID, dev.ID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	deviceID := dev.ID
	contactID := contactEntity.ID
	wamid := inbound.WAMID

	msg := &Message{
		ID:             uuid.New(),
		TeamID:         dev.TeamID,
		ConversationID: conv.ID,
		DeviceID:       &deviceID,
		ContactID:      &contactID,
		WAMID:          &wamid,
		Direction:      DirectionInbound,
		Content:        inbound.Content,
		SenderPhone:    normalizedPhone,
		SenderName:     contactEntity.Name,
		SentAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.messages.Create(ctx, msg); err != nil {
		return err
	}
	if err := s.conversations.TouchFromMessage(ctx, conv, inbound.Content, true); err != nil {
		return err
	}

	resp := ToResponse(msg)
	s.publishMessage(ctx, dev.TeamID, resp)
	return s.conversations.PublishConversation(ctx, dev.TeamID, "conversation.updated", conv.ID)
}

func (s *Service) publishMessage(ctx context.Context, teamID uuid.UUID, msg Response) {
	event := Event{Type: "message.created", Data: msg}
	_ = s.ws.PublishTenant(ctx, teamID.String(), event)
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func deref(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}
