package inbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	EventConversationCreated   = "conversation.created"
	EventConversationUpdated   = "conversation.updated"
	EventConversationAssigned  = "conversation.assigned"
	EventConversationReassigned = "conversation.reassigned"
	EventConversationClosed    = "conversation.closed"
	EventConversationReopened  = "conversation.reopened"
	EventConversationDeleted   = "conversation.deleted"
	EventConversationUnreadChanged = "conversation.unread_changed"
	EventConversationNoteCreated = "conversation.note.created"
	EventMessageCreated      = "message.created"
	EventMessageStatusUpdate = "message.status.updated"
	EventAssignmentUpdated   = "assignment.updated"
	EventConversationOpened  = "conversation.opened"
	EventConversationViewClosed = "conversation.view.closed"
	EventAgentTypingStart    = "agent.typing.start"
	EventAgentTypingStop     = "agent.typing.stop"
)

// RealtimeEvent is broadcast to team WebSocket clients.
type RealtimeEvent struct {
	Type      string          `json:"type"`
	TeamID    string          `json:"team_id"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// RealtimePublisher fans out inbox events to connected agents.
type RealtimePublisher interface {
	PublishTeamEvent(ctx context.Context, teamID uuid.UUID, event RealtimeEvent) error
}

func marshalData(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
