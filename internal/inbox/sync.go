package inbox

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

// InboundParams is input for synchronizing an inbound message into the inbox.
type InboundParams struct {
	TeamID         uuid.UUID
	Conversation   *conversation.Conversation
	ConversationCreated bool
	Message        *message.Message
	MessageCreated bool
	Preview        string
}

// OutboundParams is input for synchronizing an agent reply.
type OutboundParams struct {
	TeamID       uuid.UUID
	AgentID      uuid.UUID
	Conversation *conversation.Conversation
	Message      *message.Message
}

// CollaborationRecorder persists inbox timeline entries.
type CollaborationRecorder interface {
	RecordStatusChange(ctx context.Context, input conversation.StatusActivityInput) error
}

// SyncService coordinates inbox state and realtime projections.
type SyncService struct {
	conversations conversation.Repository
	settings      SettingsRepository
	assignment    *AssignmentEngine
	realtime      RealtimePublisher
	collaboration CollaborationRecorder
}

func NewSyncService(
	conversations conversation.Repository,
	settings SettingsRepository,
	assignment *AssignmentEngine,
	realtime RealtimePublisher,
) *SyncService {
	return &SyncService{
		conversations: conversations,
		settings:      settings,
		assignment:    assignment,
		realtime:      realtime,
	}
}

func (s *SyncService) SetCollaboration(collaboration CollaborationRecorder) {
	s.collaboration = collaboration
}

func (s *SyncService) HandleInbound(ctx context.Context, p InboundParams) (*conversation.Conversation, error) {
	now := time.Now().UTC()
	if p.Message != nil && !p.Message.ReceivedAt.IsZero() {
		now = p.Message.ReceivedAt
	}
	preview := strings.TrimSpace(p.Preview)
	if preview == "" {
		preview = "New message"
	}

	conv := p.Conversation
	status := NextStatusOnInbound(conv.Status)
	reopen := ShouldReopen(conv.Status)
	unread := conv.UnreadCount
	if p.MessageCreated {
		unread++
	}

	update := conversation.InboxSyncUpdate{
		Status:                status,
		UnreadCount:           unread,
		LastMessagePreview:    preview,
		LastMessageAt:         now,
		LastCustomerMessageAt: &now,
		ClearResolutionAt:     reopen,
		TouchLastMessage:      true,
	}
	if err := s.conversations.ApplyInboxSync(ctx, p.TeamID, conv.ID, update); err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindByID(ctx, p.TeamID, conv.ID)
	if err != nil {
		return nil, err
	}

	if reopen && s.collaboration != nil {
		_ = s.collaboration.RecordStatusChange(ctx, conversation.StatusActivityInput{
			TeamID:         p.TeamID,
			ConversationID: conv.ID,
			Previous:       p.Conversation.Status,
			Next:           conv.Status,
			Automatic:      true,
		})
	}

	if p.ConversationCreated && conv.AssignedTo == nil && s.assignment != nil {
		if assignee, err := s.assignment.PickAssignee(ctx, p.TeamID); err == nil && assignee != nil {
			if err := s.conversations.UpdateAssignment(ctx, p.TeamID, conv.ID, assignee); err == nil {
				conv, _ = s.conversations.FindByID(ctx, p.TeamID, conv.ID)
				_ = s.publishAssignment(ctx, p.TeamID, conv)
			}
		}
	}

	if p.ConversationCreated {
		_ = s.publishConversation(ctx, p.TeamID, EventConversationCreated, conv)
	} else {
		_ = s.publishConversation(ctx, p.TeamID, EventConversationUpdated, conv)
	}
	if p.Message != nil && p.MessageCreated {
		_ = s.publishMessage(ctx, p.TeamID, EventMessageCreated, p.Message)
	}
	return conv, nil
}

func (s *SyncService) HandleOutbound(ctx context.Context, p OutboundParams) (*conversation.Conversation, error) {
	now := time.Now().UTC()
	settings, _ := s.settings.Get(ctx, p.TeamID)
	unread := p.Conversation.UnreadCount
	if settings != nil && settings.ResetUnreadOnReply {
		unread = 0
	}

	status := p.Conversation.Status
	reopen := ShouldReopen(status)
	if reopen {
		status = conversation.StatusOpen
	}

	setFirstResponse := p.Conversation.FirstResponseAt == nil
	update := conversation.InboxSyncUpdate{
		Status:             status,
		UnreadCount:        unread,
		LastAgentMessageAt: &now,
		SetFirstResponseAt: setFirstResponse,
		LastMessagePreview: p.Message.Content,
		LastMessageAt:      now,
		TouchLastMessage:   true,
		ClearResolutionAt:  reopen,
	}
	if err := s.conversations.ApplyInboxSync(ctx, p.TeamID, p.Conversation.ID, update); err != nil {
		return nil, err
	}
	conv, err := s.conversations.FindByID(ctx, p.TeamID, p.Conversation.ID)
	if err != nil {
		return nil, err
	}
	if reopen && s.collaboration != nil {
		_ = s.collaboration.RecordStatusChange(ctx, conversation.StatusActivityInput{
			TeamID:         p.TeamID,
			ConversationID: conv.ID,
			Previous:       p.Conversation.Status,
			Next:           conv.Status,
			Automatic:      true,
		})
	}
	_ = s.publishMessage(ctx, p.TeamID, EventMessageCreated, p.Message)
	_ = s.publishConversation(ctx, p.TeamID, EventConversationUpdated, conv)
	return conv, nil
}

func (s *SyncService) MarkRead(ctx context.Context, teamID, conversationID uuid.UUID) (*conversation.Conversation, error) {
	if err := s.conversations.MarkRead(ctx, teamID, conversationID); err != nil {
		return nil, err
	}
	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	_ = s.publishConversation(ctx, teamID, EventConversationUpdated, conv)
	return conv, nil
}

func (s *SyncService) OnStatusChange(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation) error {
	return s.publishConversation(ctx, teamID, EventConversationUpdated, conv)
}

func (s *SyncService) OnAssignment(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation) error {
	_ = s.publishAssignment(ctx, teamID, conv)
	return s.publishConversation(ctx, teamID, EventConversationUpdated, conv)
}

func (s *SyncService) publishConversation(ctx context.Context, teamID uuid.UUID, eventType string, conv *conversation.Conversation) error {
	if s.realtime == nil || conv == nil {
		return nil
	}
	return s.realtime.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type:   eventType,
		TeamID: teamID.String(),
		Data:   marshalData(conv),
	})
}

func (s *SyncService) publishMessage(ctx context.Context, teamID uuid.UUID, eventType string, msg *message.Message) error {
	if s.realtime == nil || msg == nil {
		return nil
	}
	return s.realtime.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type:   eventType,
		TeamID: teamID.String(),
		Data:   marshalData(msg),
	})
}

func (s *SyncService) publishAssignment(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation) error {
	if s.realtime == nil || conv == nil {
		return nil
	}
	return s.realtime.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type:   EventAssignmentUpdated,
		TeamID: teamID.String(),
		Data: marshalData(struct {
			ConversationID uuid.UUID  `json:"conversation_id"`
			AssignedTo     *uuid.UUID `json:"assigned_to"`
		}{ConversationID: conv.ID, AssignedTo: conv.AssignedTo}),
	})
}
