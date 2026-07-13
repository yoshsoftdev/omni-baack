package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (ConversationService) manages inbox conversations.
type Service struct {
	conversations Repository
	contacts      contact.Repository
	teams         team.Repository
	rbac          *rbac.Service
	inbox         InboxSync
	collaboration *CollaborationService
}

// InboxSync publishes inbox side-effects (realtime, assignment).
type InboxSync interface {
	OnStatusChange(ctx context.Context, teamID uuid.UUID, conv *Conversation) error
	OnAssignment(ctx context.Context, teamID uuid.UUID, conv *Conversation) error
	MarkRead(ctx context.Context, teamID, conversationID uuid.UUID) (*Conversation, error)
}

func NewService(
	conversations Repository,
	contacts contact.Repository,
	teams team.Repository,
	rbacSvc *rbac.Service,
) *Service {
	return &Service{
		conversations: conversations,
		contacts:      contacts,
		teams:         teams,
		rbac:          rbacSvc,
	}
}

func (s *Service) SetInboxSync(inbox InboxSync) {
	s.inbox = inbox
}

func (s *Service) SetCollaboration(collaboration *CollaborationService) {
	s.collaboration = collaboration
}

type CreateInput struct {
	ContactID   uuid.UUID
	ChannelID   *uuid.UUID
	ChannelType ChannelType
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Conversation, error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.contacts.FindByID(ctx, teamID, input.ContactID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	if existing, err := s.conversations.FindByContactChannel(ctx, teamID, input.ContactID, input.ChannelID); err == nil {
		return existing, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, serviceerrors.MapRepository(err)
	}

	channelType := input.ChannelType
	if channelType == "" {
		channelType = ChannelWhatsApp
	}

	now := time.Now().UTC()
	c := &Conversation{
		ID:          uuid.New(),
		TeamID:      teamID,
		ContactID:   input.ContactID,
		ChannelID:   input.ChannelID,
		ChannelType: channelType,
		Status:      StatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := serviceerrors.MapRepository(s.conversations.Create(ctx, c)); err != nil {
		return nil, err
	}
	if s.collaboration != nil {
		_ = s.collaboration.RecordOpened(ctx, teamID, c.ID, userID)
	}
	return c, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID) (*Item, error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	c, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return s.enrichOne(ctx, teamID, c)
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Item], error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return repository.Page[Item]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Item]{}, err
	}
	if filter.View == ViewMine {
		filter.ActorID = &userID
	}
	result, err := s.conversations.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Item]{}, serviceerrors.MapRepository(err)
	}
	return s.enrichPage(ctx, teamID, result)
}

func (s *Service) ListCursor(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, cursor *ListCursor, limit int) (CursorListPage, error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return CursorListPage{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return CursorListPage{}, err
	}
	if filter.View == ViewMine {
		filter.ActorID = &userID
	}
	result, err := s.conversations.ListCursor(ctx, teamID, filter, cursor, limit)
	if err != nil {
		return CursorListPage{}, serviceerrors.MapRepository(err)
	}
	contactMap, err := s.loadContactsByConversations(ctx, teamID, result.Items)
	if err != nil {
		return CursorListPage{}, err
	}
	return CursorListPage{
		Items:      toItems(result.Items, contactMap),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

// CursorListPage is the enriched cursor page for inbox API responses.
type CursorListPage struct {
	Items      []Item
	NextCursor *ListCursor
	HasMore    bool
}

func (s *Service) Assign(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, assigneeID *uuid.UUID) (*Item, error) {
	if err := s.authorize(actorRole, rbac.PermConversationAssign); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	prev, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if assigneeID != nil {
		if _, err := s.teams.FindMemberByUser(ctx, teamID, *assigneeID); err != nil {
			if err == team.ErrMemberNotFound {
				return nil, serviceerrors.NewValidation("assigned_to", "assignee must be a team member")
			}
			return nil, err
		}
	}
	if err := serviceerrors.MapRepository(s.conversations.UpdateAssignment(ctx, teamID, conversationID, assigneeID)); err != nil {
		return nil, err
	}
	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if s.collaboration != nil {
		_ = s.collaboration.RecordAssignment(ctx, AssignmentActivityInput{
			TeamID:         teamID,
			ConversationID: conversationID,
			ActorID:        userID,
			Previous:       prev.AssignedTo,
			Assignee:       assigneeID,
		})
	}
	if s.inbox != nil {
		_ = s.inbox.OnAssignment(ctx, teamID, conv)
	}
	return s.enrichOne(ctx, teamID, conv)
}

func (s *Service) UpdateStatus(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, status Status) (*Item, error) {
	if status == StatusClosed || status == StatusResolved {
		if err := s.authorize(actorRole, rbac.PermConversationClose); err != nil {
			return nil, err
		}
	} else if err := s.authorize(actorRole, rbac.PermConversationReply); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := validateStatus(status); err != nil {
		return nil, err
	}
	prev, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if status == StatusResolved || status == StatusClosed {
		if err := serviceerrors.MapRepository(s.conversations.UpdateStatus(ctx, teamID, conversationID, status)); err != nil {
			return nil, err
		}
	} else if err := serviceerrors.MapRepository(s.conversations.UpdateStatus(ctx, teamID, conversationID, status)); err != nil {
		return nil, err
	}
	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if s.collaboration != nil {
		_ = s.collaboration.RecordStatusChange(ctx, StatusActivityInput{
			TeamID:         teamID,
			ConversationID: conversationID,
			ActorID:        &userID,
			Previous:       prev.Status,
			Next:           conv.Status,
		})
	}
	if s.inbox != nil {
		_ = s.inbox.OnStatusChange(ctx, teamID, conv)
	}
	return s.enrichOne(ctx, teamID, conv)
}

func (s *Service) MarkRead(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID) (*Item, error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.conversations.FindByID(ctx, teamID, conversationID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if s.inbox != nil {
		conv, err := s.inbox.MarkRead(ctx, teamID, conversationID)
		if err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
		return s.enrichOne(ctx, teamID, conv)
	}
	if err := serviceerrors.MapRepository(s.conversations.MarkRead(ctx, teamID, conversationID)); err != nil {
		return nil, err
	}
	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return s.enrichOne(ctx, teamID, conv)
}

func (s *Service) RecordInboundPreview(ctx context.Context, teamID uuid.UUID, conversationID uuid.UUID, preview string) error {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		preview = "New message"
	}
	if len(preview) > 200 {
		preview = preview[:200]
	}
	now := time.Now().UTC()
	return serviceerrors.MapRepository(s.conversations.UpdateLastMessage(ctx, teamID, conversationID, LastMessageUpdate{
		Preview: preview,
		At:      now,
	}))
}

func validateStatus(status Status) error {
	switch status {
	case StatusOpen, StatusPending, StatusResolved, StatusClosed, StatusAssigned:
		return nil
	default:
		return serviceerrors.NewValidation("status", "invalid value")
	}
}
