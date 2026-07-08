package message

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (MessageService) manages inbox messages.
type Service struct {
	messages      Repository
	attachments   AttachmentRepository
	conversations conversation.Repository
	teams         team.Repository
	users         user.Repository
	rbac          *rbac.Service
	outbound      *OutboundDeps
}

func NewService(
	messages Repository,
	attachments AttachmentRepository,
	conversations conversation.Repository,
	teams team.Repository,
	users user.Repository,
	rbacSvc *rbac.Service,
	outbound *OutboundDeps,
) *Service {
	return &Service{
		messages:      messages,
		attachments:   attachments,
		conversations: conversations,
		teams:         teams,
		users:         users,
		rbac:          rbacSvc,
		outbound:      outbound,
	}
}

type CreateInput struct {
	ConversationID uuid.UUID
	ChannelID      *uuid.UUID
	Direction      Direction
	Sender         string
	Recipient      *string
	Content        string
	ContentType    ContentType
	WAMID          *string
	Status         *Status
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

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Message, error) {
	perm := rbac.PermConversationReply
	if input.Direction == DirectionInbound {
		perm = rbac.PermConversationView
	}
	if err := s.authorize(actorRole, perm); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindByID(ctx, teamID, input.ConversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, serviceerrors.NewValidation("content", "is required")
	}
	sender := strings.TrimSpace(input.Sender)
	if sender == "" {
		return nil, serviceerrors.NewValidation("sender", "is required")
	}
	contentType := input.ContentType
	if contentType == "" {
		contentType = ContentText
	}

	now := time.Now().UTC()
	m := &Message{
		ID:             uuid.New(),
		ConversationID: input.ConversationID,
		TeamID:         teamID,
		ChannelID:      input.ChannelID,
		Direction:      input.Direction,
		Sender:         sender,
		Recipient:      input.Recipient,
		Content:        content,
		ContentType:    contentType,
		WAMID:          input.WAMID,
		Status:         input.Status,
		ReceivedAt:     now,
		CreatedAt:      now,
	}
	if m.ChannelID == nil {
		m.ChannelID = conv.ChannelID
	}
	if err := serviceerrors.MapRepository(s.messages.Create(ctx, m)); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) ListByConversation(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Message], error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return repository.Page[Message]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Message]{}, err
	}
	if _, err := s.conversations.FindByID(ctx, teamID, conversationID); err != nil {
		return repository.Page[Message]{}, serviceerrors.MapRepository(err)
	}
	result, err := s.messages.ListByConversation(ctx, teamID, conversationID, filter, page)
	if err != nil {
		return repository.Page[Message]{}, serviceerrors.MapRepository(err)
	}
	if s.attachments != nil && len(result.Items) > 0 {
		ids := make([]uuid.UUID, 0, len(result.Items))
		for i := range result.Items {
			ids = append(ids, result.Items[i].ID)
		}
		attachmentMap, attachErr := s.attachments.FindByMessageIDs(ctx, teamID, ids)
		if attachErr != nil {
			return repository.Page[Message]{}, serviceerrors.MapRepository(attachErr)
		}
		for i := range result.Items {
			if attachment, ok := attachmentMap[result.Items[i].ID]; ok {
				result.Items[i].Attachment = attachment
			}
		}
	}
	if err := s.enrichSenderNames(ctx, result.Items); err != nil {
		return repository.Page[Message]{}, err
	}
	return result, nil
}

func (s *Service) enrichSenderNames(ctx context.Context, items []Message) error {
	if s.users == nil {
		return nil
	}
	cache := make(map[string]string)
	for i := range items {
		if items[i].Direction != DirectionOutbound {
			continue
		}
		senderID := strings.TrimSpace(items[i].Sender)
		if senderID == "" {
			continue
		}
		if name, ok := cache[senderID]; ok {
			if name != "" {
				items[i].SenderName = &name
			}
			continue
		}
		id, err := uuid.Parse(senderID)
		if err != nil {
			cache[senderID] = ""
			continue
		}
		u, err := s.users.FindByID(ctx, id)
		if err != nil || u == nil {
			cache[senderID] = ""
			continue
		}
		name := strings.TrimSpace(u.Name)
		cache[senderID] = name
		if name != "" {
			items[i].SenderName = &name
		}
	}
	return nil
}

func (s *Service) UpdateDeliveryStatus(ctx context.Context, wamid string, status Status) error {
	if strings.TrimSpace(wamid) == "" {
		return serviceerrors.NewValidation("wamid", "is required")
	}
	m, err := s.messages.FindByWAMID(ctx, wamid)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	return serviceerrors.MapRepository(s.messages.UpdateStatus(ctx, m.TeamID, m.ID, status))
}

func (s *Service) MarkDeletedByWAMID(ctx context.Context, teamID uuid.UUID, wamid string, source DeletedBySource) error {
	wamid = strings.TrimSpace(wamid)
	if wamid == "" {
		return serviceerrors.NewValidation("wamid", "is required")
	}
	return serviceerrors.MapRepository(s.messages.MarkDeletedByWAMID(ctx, teamID, wamid, source))
}

func (s *Service) DeleteByID(ctx context.Context, teamID, userID uuid.UUID, actorRole string, id uuid.UUID, source DeletedBySource) (*Message, error) {
	if err := s.authorize(actorRole, rbac.PermConversationReply); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	m, err := s.messages.FindByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if err := s.messages.MarkDeletedByID(ctx, teamID, id, source); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if source == DeletedBySourceAgent {
		_ = s.revokeOnProvider(ctx, teamID, m)
	}
	updated, err := s.messages.FindByID(ctx, teamID, id)
	if err == nil {
		return updated, nil
	}
	m.IsDeleted = true
	now := time.Now().UTC()
	m.DeletedAt = &now
	src := source
	m.DeletedBySource = &src
	return m, nil
}
