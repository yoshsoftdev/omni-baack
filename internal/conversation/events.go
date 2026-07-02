package conversation

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

const (
	EventAssigned   = "conversation.assigned"
	EventReassigned = "conversation.reassigned"
	EventClosed     = "conversation.closed"
	EventReopened   = "conversation.reopened"
	EventNoteCreated = "conversation.note.created"
)

// EventPublisher broadcasts collaboration events to connected clients.
type EventPublisher interface {
	PublishConversationEvent(ctx context.Context, teamID uuid.UUID, eventType string, payload any) error
}

type hubEventPublisher struct {
	publish func(ctx context.Context, teamID uuid.UUID, eventType string, data json.RawMessage) error
}

func NewHubEventPublisher(publish func(ctx context.Context, teamID uuid.UUID, eventType string, data json.RawMessage) error) EventPublisher {
	return &hubEventPublisher{publish: publish}
}

func (p *hubEventPublisher) PublishConversationEvent(ctx context.Context, teamID uuid.UUID, eventType string, payload any) error {
	if p == nil || p.publish == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.publish(ctx, teamID, eventType, data)
}
