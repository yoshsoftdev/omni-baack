package inbox

import (
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

// ListService serves cursor-paginated inbox queries.
type ListService struct {
	conversations *conversation.Service
}

func NewListService(conversations *conversation.Service) *ListService {
	return &ListService{conversations: conversations}
}

type ListInput struct {
	Filter ListFilterInput
	Cursor string
	Limit  int
}

type ListFilterInput struct {
	Status     *conversation.Status
	AssignedTo *uuid.UUID
	ChannelID  *uuid.UUID
	ChannelType *conversation.ChannelType
	Search     string
	View       conversation.InboxView
}

type ListResult struct {
	Conversations []conversation.Item `json:"conversations"`
	NextCursor    string              `json:"next_cursor,omitempty"`
	HasMore       bool                `json:"has_more"`
}

func (s *ListService) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input ListInput) (*ListResult, error) {
	cursor, err := DecodeCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	filter := conversation.ListFilter{
		Status:      input.Filter.Status,
		AssignedTo:  input.Filter.AssignedTo,
		ChannelID:   input.Filter.ChannelID,
		ChannelType: input.Filter.ChannelType,
		Search:      input.Filter.Search,
		View:        input.Filter.View,
	}

	page, err := s.conversations.ListCursor(ctx, teamID, userID, actorRole, filter, cursor, limit)
	if err != nil {
		return nil, err
	}

	nextCursor := ""
	if page.NextCursor != nil {
		nextCursor, err = EncodeCursor(page.NextCursor)
		if err != nil {
			return nil, err
		}
	}

	return &ListResult{
		Conversations: page.Items,
		NextCursor:    nextCursor,
		HasMore:       page.HasMore,
	}, nil
}
