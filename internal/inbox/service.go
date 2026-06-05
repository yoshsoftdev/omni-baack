package inbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var (
	ErrInvalidContent    = errors.New("content is required")
	ErrInvalidRecipient  = errors.New("recipient is required")
	ErrDeviceNotConnected = errors.New("device is not connected")
	ErrDeviceNotFound    = errors.New("device not found")
)

type Service struct {
	repo    Repository
	devices device.Repository
	meta    *metaclient.Client
	ws      *hub.Hub
}

func NewService(repo Repository, devices device.Repository, meta *metaclient.Client, wsHub *hub.Hub) *Service {
	return &Service{
		repo:    repo,
		devices: devices,
		meta:    meta,
		ws:      wsHub,
	}
}

func (s *Service) List(ctx context.Context, teamID uuid.UUID) ([]Message, error) {
	return s.repo.ListByTeam(ctx, teamID, 100)
}

type SendInput struct {
	DeviceID  uuid.UUID
	Recipient string
	Content   string
}

func (s *Service) Send(ctx context.Context, teamID uuid.UUID, input SendInput) (*Message, error) {
	content := strings.TrimSpace(input.Content)
	recipient := strings.TrimSpace(input.Recipient)
	if content == "" {
		return nil, ErrInvalidContent
	}
	if recipient == "" {
		return nil, ErrInvalidRecipient
	}

	dev, err := s.devices.FindByID(ctx, teamID, input.DeviceID)
	if err != nil {
		return nil, ErrDeviceNotFound
	}
	if dev.Status != device.StatusConnected || dev.AccessToken == nil || dev.PhoneNumberID == nil {
		return nil, ErrDeviceNotConnected
	}

	wamid, err := s.meta.SendTextMessage(ctx, *dev.PhoneNumberID, *dev.AccessToken, recipient, content)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	deviceID := dev.ID
	wamidStr := wamid
	msg := &Message{
		ID:         uuid.New(),
		TeamID:     teamID,
		DeviceID:   &deviceID,
		WAMID:      &wamidStr,
		Sender:     deref(dev.PhoneNumber, "system"),
		Recipient:  &recipient,
		Content:    content,
		Direction:  DirectionOutbound,
		ReceivedAt: now,
	}

	if err := s.repo.Save(ctx, msg); err != nil {
		return nil, err
	}
	s.publish(ctx, teamID, msg)
	return msg, nil
}

func (s *Service) HandleMetaInbound(ctx context.Context, inbound metaclient.InboundMessage) error {
	dev, err := s.devices.FindByPhoneNumberID(ctx, inbound.PhoneNumberID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	deviceID := dev.ID
	wamid := inbound.WAMID
	msg := &Message{
		ID:         uuid.New(),
		TeamID:     dev.TeamID,
		DeviceID:   &deviceID,
		WAMID:      &wamid,
		Sender:     inbound.From,
		Content:    inbound.Content,
		Direction:  DirectionInbound,
		ReceivedAt: now,
	}

	if err := s.repo.Save(ctx, msg); err != nil {
		return err
	}
	s.publish(ctx, dev.TeamID, msg)
	return nil
}

func (s *Service) publish(ctx context.Context, teamID uuid.UUID, msg *Message) {
	event := Event{Type: "inbox.message", Data: *msg}
	_ = s.ws.PublishTenant(ctx, teamID.String(), event)
}

func deref(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}
