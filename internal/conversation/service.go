package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
)

var (
	ErrForbidden        = errors.New("insufficient permissions")
	ErrInvalidStatus    = errors.New("invalid status")
	ErrAssigneeNotFound = errors.New("assignee is not a team member")
)

type Service struct {
	conversations Repository
	teams         team.Repository
	ws            *hub.Hub
}

func NewService(conversations Repository, teams team.Repository, wsHub *hub.Hub) *Service {
	return &Service{conversations: conversations, teams: teams, ws: wsHub}
}

type ListInput struct {
	Status     *Status
	AssignedTo *uuid.UUID
	Search     string
	Limit      int
}

func (s *Service) UnreadCount(ctx context.Context, teamID, userID uuid.UUID) (int, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return 0, err
	}
	return s.conversations.SumUnreadCount(ctx, teamID)
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, input ListInput) ([]ListItem, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.conversations.List(ctx, teamID, ListFilter{
		Status:     input.Status,
		AssignedTo: input.AssignedTo,
		Search:     input.Search,
		Limit:      input.Limit,
	})
}

func (s *Service) Get(ctx context.Context, teamID, userID, conversationID uuid.UUID) (*Detail, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.conversations.GetDetail(ctx, teamID, conversationID)
}

func (s *Service) Assign(ctx context.Context, teamID, userID uuid.UUID, role team.Role, conversationID uuid.UUID, assigneeID *uuid.UUID) (*Detail, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}

	if assigneeID != nil {
		if _, err := s.teams.FindMemberByUser(ctx, teamID, *assigneeID); err != nil {
			return nil, ErrAssigneeNotFound
		}
	}

	conv.AssignedTo = assigneeID
	conv.UpdatedAt = time.Now().UTC()
	if err := s.conversations.Update(ctx, conv); err != nil {
		return nil, err
	}

	detail, err := s.conversations.GetDetail(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	s.publish(ctx, teamID, "conversation.updated", detail)
	return detail, nil
}

func (s *Service) UpdateStatus(ctx context.Context, teamID, userID uuid.UUID, conversationID uuid.UUID, status Status) (*Detail, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if status != StatusOpen && status != StatusPending && status != StatusResolved {
		return nil, ErrInvalidStatus
	}

	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}

	conv.Status = status
	conv.UpdatedAt = time.Now().UTC()
	if status == StatusResolved {
		conv.UnreadCount = 0
	}
	if err := s.conversations.Update(ctx, conv); err != nil {
		return nil, err
	}

	detail, err := s.conversations.GetDetail(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	s.publish(ctx, teamID, "conversation.updated", detail)
	return detail, nil
}

func (s *Service) MarkRead(ctx context.Context, teamID, userID, conversationID uuid.UUID) (*Detail, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.UnreadCount == 0 {
		return s.conversations.GetDetail(ctx, teamID, conversationID)
	}

	conv.UnreadCount = 0
	conv.UpdatedAt = time.Now().UTC()
	if err := s.conversations.Update(ctx, conv); err != nil {
		return nil, err
	}

	detail, err := s.conversations.GetDetail(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	s.publish(ctx, teamID, "conversation.updated", detail)
	return detail, nil
}

func (s *Service) FindOrCreate(ctx context.Context, teamID, contactID, deviceID uuid.UUID) (*Conversation, error) {
	existing, err := s.conversations.FindByContactDevice(ctx, teamID, contactID, deviceID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	conv := &Conversation{
		ID:        uuid.New(),
		TeamID:    teamID,
		ContactID: contactID,
		DeviceID:  deviceID,
		Status:    StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.conversations.Create(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *Service) TouchFromMessage(ctx context.Context, conv *Conversation, content string, inbound bool) error {
	now := time.Now().UTC()
	preview := strings.TrimSpace(content)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	conv.LastMessageAt = &now
	conv.LastMessagePreview = &preview
	conv.UpdatedAt = now
	if inbound {
		conv.UnreadCount++
		if conv.Status == StatusResolved {
			conv.Status = StatusOpen
		}
	}
	return s.conversations.Update(ctx, conv)
}

func (s *Service) PublishConversation(ctx context.Context, teamID uuid.UUID, eventType string, conversationID uuid.UUID) error {
	detail, err := s.conversations.GetDetail(ctx, teamID, conversationID)
	if err != nil {
		return err
	}
	s.publish(ctx, teamID, eventType, detail)
	return nil
}

func (s *Service) publish(ctx context.Context, teamID uuid.UUID, eventType string, data interface{}) {
	event := Event{Type: eventType, Data: data}
	_ = s.ws.PublishTenant(ctx, teamID.String(), event)
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin || role == team.RoleAgent
}
