package inbox

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

// InboundParams is input for synchronizing an inbound message into the inbox.
type InboundParams struct {
	TeamID              uuid.UUID
	Conversation        *conversation.Conversation
	ConversationCreated bool
	Message             *message.Message
	MessageCreated      bool
	Preview             string
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
	contacts      contact.Repository
	settings      SettingsRepository
	assignment    *AssignmentEngine
	realtime      RealtimePublisher
	collaboration CollaborationRecorder
}

func NewSyncService(
	conversations conversation.Repository,
	contacts contact.Repository,
	settings SettingsRepository,
	assignment *AssignmentEngine,
	realtime RealtimePublisher,
) *SyncService {
	return &SyncService{
		conversations: conversations,
		contacts:      contacts,
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

	working := *conv
	working.Status = status
	working.UnreadCount = unread
	working.LastCustomerMessageAt = &now
	priority := RecalculatePriority(&working, PriorityEventInbound, now)

	update := conversation.InboxSyncUpdate{
		Status:                status,
		UnreadCount:           unread,
		LastMessagePreview:    preview,
		LastMessageAt:         now,
		LastCustomerMessageAt: &now,
		ClearResolutionAt:     reopen,
		TouchLastMessage:      true,
		WaitingSince:          priority.WaitingSince,
		BasePriority:          &priority.BasePriority,
		PriorityScore:         &priority.PriorityScore,
	}
	if err := s.conversations.ApplyInboxSync(ctx, p.TeamID, conv.ID, update); err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindByID(ctx, p.TeamID, conv.ID)
	if err != nil {
		return nil, err
	}
	if reopen && conv.AssignedTo != nil {
		if err := s.conversations.UpdateAssignment(ctx, p.TeamID, conv.ID, nil); err == nil {
			conv, _ = s.conversations.FindByID(ctx, p.TeamID, conv.ID)
			_ = s.publishAssignment(ctx, p.TeamID, conv)
		}
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
				_ = s.refreshPriority(ctx, p.TeamID, conv, PriorityEventAssignment)
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
	working := *p.Conversation
	working.Status = status
	working.UnreadCount = unread
	working.LastAgentMessageAt = &now
	priority := RecalculatePriority(&working, PriorityEventOutbound, now)

	update := conversation.InboxSyncUpdate{
		Status:             status,
		UnreadCount:        unread,
		LastAgentMessageAt: &now,
		SetFirstResponseAt: setFirstResponse,
		LastMessagePreview: p.Message.Content,
		LastMessageAt:      now,
		TouchLastMessage:   true,
		ClearResolutionAt:  reopen,
		ClearWaitingSince:  true,
		BasePriority:       &priority.BasePriority,
		PriorityScore:      &priority.PriorityScore,
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
	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.UnreadCount == 0 {
		return conv, nil
	}

	now := time.Now().UTC()
	working := *conv
	working.UnreadCount = 0
	priority := RecalculatePriority(&working, PriorityEventMarkRead, now)

	if err := s.conversations.MarkRead(ctx, teamID, conversationID); err != nil {
		return nil, err
	}
	if err := s.conversations.UpdatePriorityFields(ctx, teamID, conversationID, priority.WaitingSince, false, priority.BasePriority, priority.PriorityScore); err != nil {
		return nil, err
	}

	conv, err = s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	_ = s.publishConversation(ctx, teamID, EventConversationUnreadChanged, conv)
	return conv, nil
}

func (s *SyncService) OnStatusChange(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation) error {
	event := PriorityEventAssignment
	if conv.Status == conversation.StatusClosed || conv.Status == conversation.StatusResolved {
		event = PriorityEventClose
	} else if conv.Status == conversation.StatusOpen || conv.Status == conversation.StatusPending {
		event = PriorityEventReopen
	}
	if err := s.refreshPriority(ctx, teamID, conv, event); err != nil {
		return err
	}
	conv, err := s.conversations.FindByID(ctx, teamID, conv.ID)
	if err != nil {
		return err
	}
	return s.publishConversation(ctx, teamID, EventConversationUpdated, conv)
}

func (s *SyncService) OnAssignment(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation) error {
	if err := s.refreshPriority(ctx, teamID, conv, PriorityEventAssignment); err != nil {
		return err
	}
	conv, err := s.conversations.FindByID(ctx, teamID, conv.ID)
	if err != nil {
		return err
	}
	_ = s.publishAssignment(ctx, teamID, conv)
	return s.publishConversation(ctx, teamID, EventConversationUpdated, conv)
}

func (s *SyncService) OnDeleted(ctx context.Context, teamID uuid.UUID, conversationID uuid.UUID) error {
	if s.realtime == nil {
		return nil
	}
	return s.realtime.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type: EventConversationDeleted,
		Data: marshalData(struct {
			ConversationID uuid.UUID `json:"conversation_id"`
		}{ConversationID: conversationID}),
	})
}

func (s *SyncService) refreshPriority(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation, event PriorityEvent) error {
	now := time.Now().UTC()
	priority := RecalculatePriority(conv, event, now)
	clearWaiting := priority.WaitingSince == nil
	return s.conversations.UpdatePriorityFields(ctx, teamID, conv.ID, priority.WaitingSince, clearWaiting, priority.BasePriority, priority.PriorityScore)
}

func (s *SyncService) publishConversation(ctx context.Context, teamID uuid.UUID, eventType string, conv *conversation.Conversation) error {
	if s.realtime == nil || conv == nil {
		return nil
	}
	item := s.conversationItem(ctx, teamID, conv)
	payload := ToDelta(item)
	if eventType == EventConversationCreated {
		payload = ToCreatedPayload(item)
	}
	return s.realtime.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type:   eventType,
		TeamID: teamID.String(),
		Data:   marshalData(payload),
	})
}

func (s *SyncService) conversationItem(ctx context.Context, teamID uuid.UUID, conv *conversation.Conversation) conversation.Item {
	if s.contacts == nil {
		return conversation.ToRealtimeItem(conv, nil)
	}
	ct, err := s.contacts.FindByID(ctx, teamID, conv.ContactID)
	if err != nil {
		return conversation.ToRealtimeItem(conv, nil)
	}
	return conversation.ToRealtimeItem(conv, ct)
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
	item := s.conversationItem(ctx, teamID, conv)
	delta := ToDelta(item)
	return s.realtime.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type:   EventConversationAssigned,
		TeamID: teamID.String(),
		Data:   marshalData(delta),
	})
}
