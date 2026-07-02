package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// CollaborationService records inbox activity and internal notes.
type CollaborationService struct {
	activities    ActivityRepository
	notes         NoteRepository
	conversations Repository
	teams         team.Repository
	users         user.Repository
	rbac          *rbac.Service
	realtime      EventPublisher
}

func NewCollaborationService(
	activities ActivityRepository,
	notes NoteRepository,
	conversations Repository,
	teams team.Repository,
	users user.Repository,
	rbacSvc *rbac.Service,
	realtime EventPublisher,
) *CollaborationService {
	return &CollaborationService{
		activities:    activities,
		notes:         notes,
		conversations: conversations,
		teams:         teams,
		users:         users,
		rbac:          rbacSvc,
		realtime:      realtime,
	}
}

func (s *CollaborationService) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *CollaborationService) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *CollaborationService) actorName(ctx context.Context, actorID *uuid.UUID) string {
	if actorID == nil {
		return ""
	}
	u, err := s.users.FindByID(ctx, *actorID)
	if err != nil || u == nil {
		return ""
	}
	return strings.TrimSpace(u.Name)
}

func (s *CollaborationService) memberName(ctx context.Context, _, userID uuid.UUID) string {
	return s.actorName(ctx, &userID)
}

func (s *CollaborationService) ListActivities(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID) ([]Activity, error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.conversations.FindByID(ctx, teamID, conversationID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return s.activities.ListByConversation(ctx, teamID, conversationID, 100)
}

func (s *CollaborationService) ListNotes(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID) ([]Note, error) {
	if err := s.authorize(actorRole, rbac.PermConversationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.conversations.FindByID(ctx, teamID, conversationID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return s.notes.ListByConversation(ctx, teamID, conversationID)
}

type CreateNoteInput struct {
	Content string
}

func (s *CollaborationService) CreateNote(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, input CreateNoteInput) (*Note, error) {
	if err := s.authorize(actorRole, rbac.PermConversationNote); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, serviceerrors.NewValidation("content", "is required")
	}
	if _, err := s.conversations.FindByID(ctx, teamID, conversationID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	now := time.Now().UTC()
	authorName := s.actorName(ctx, &userID)
	note := &Note{
		ID:             uuid.New(),
		TeamID:         teamID,
		ConversationID: conversationID,
		AuthorID:       userID,
		AuthorName:     authorName,
		Content:        content,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := serviceerrors.MapRepository(s.notes.Create(ctx, note)); err != nil {
		return nil, err
	}

	activity, err := s.recordActivity(ctx, recordActivityInput{
		TeamID:         teamID,
		ConversationID: conversationID,
		Type:           ActivityNoteAdded,
		ActorID:        &userID,
		Description:    fmt.Sprintf("Internal note added by %s", displayName(authorName, "Agent")),
	})
	if err != nil {
		return nil, err
	}
	_ = activity
	_ = s.publishNote(ctx, teamID, note)
	return note, nil
}

func (s *CollaborationService) DeleteNote(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID, noteID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermConversationNote); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	if _, err := s.conversations.FindByID(ctx, teamID, conversationID); err != nil {
		return serviceerrors.MapRepository(err)
	}
	return serviceerrors.MapRepository(s.notes.SoftDelete(ctx, teamID, noteID))
}

type AssignmentActivityInput struct {
	TeamID         uuid.UUID
	ConversationID uuid.UUID
	ActorID        uuid.UUID
	Previous       *uuid.UUID
	Assignee       *uuid.UUID
}

func (s *CollaborationService) RecordAssignment(ctx context.Context, input AssignmentActivityInput) error {
	actorName := s.actorName(ctx, &input.ActorID)
	var assigneeName string
	if input.Assignee != nil {
		assigneeName = s.memberName(ctx, input.TeamID, *input.Assignee)
	}

	var activityType ActivityType
	var description string
	var eventType string

	switch {
	case input.Previous == nil && input.Assignee != nil:
		activityType = ActivityAssigned
		eventType = EventAssigned
		description = fmt.Sprintf("Assigned to %s by %s", displayName(assigneeName, "agent"), displayName(actorName, "Agent"))
	case input.Previous != nil && input.Assignee != nil && *input.Previous != *input.Assignee:
		prevName := s.memberName(ctx, input.TeamID, *input.Previous)
		activityType = ActivityReassigned
		eventType = EventReassigned
		description = fmt.Sprintf("Reassigned from %s to %s", displayName(prevName, "agent"), displayName(assigneeName, "agent"))
	case input.Previous != nil && input.Assignee == nil:
		prevName := s.memberName(ctx, input.TeamID, *input.Previous)
		activityType = ActivityReassigned
		eventType = EventReassigned
		description = fmt.Sprintf("Reassigned from %s to unassigned by %s", displayName(prevName, "agent"), displayName(actorName, "Agent"))
	default:
		return nil
	}

	meta, _ := json.Marshal(map[string]any{
		"assignee_id":          uuidPtrString(input.Assignee),
		"previous_assignee_id": uuidPtrString(input.Previous),
	})
	activity, err := s.recordActivity(ctx, recordActivityInput{
		TeamID:         input.TeamID,
		ConversationID: input.ConversationID,
		Type:           activityType,
		ActorID:        &input.ActorID,
		Description:    description,
		Metadata:       meta,
	})
	if err != nil {
		return err
	}
	_ = s.publishActivity(ctx, input.TeamID, eventType, activity)
	return nil
}

type StatusActivityInput struct {
	TeamID         uuid.UUID
	ConversationID uuid.UUID
	ActorID        *uuid.UUID
	Previous       Status
	Next           Status
	Automatic      bool
}

func (s *CollaborationService) RecordStatusChange(ctx context.Context, input StatusActivityInput) error {
	if input.Previous == input.Next {
		return nil
	}

	actorName := s.actorName(ctx, input.ActorID)
	var activityType ActivityType
	var description string
	var eventType string

	switch input.Next {
	case StatusClosed, StatusResolved:
		activityType = ActivityClosed
		eventType = EventClosed
		if input.Automatic {
			return nil
		}
		description = fmt.Sprintf("Conversation closed by %s", displayName(actorName, "Agent"))
	case StatusOpen:
		if input.Previous != StatusClosed && input.Previous != StatusResolved {
			if input.Automatic {
				return nil
			}
			activityType = ActivityOpened
			eventType = ""
			description = fmt.Sprintf("Conversation opened by %s", displayName(actorName, "Agent"))
		} else {
			activityType = ActivityReopened
			eventType = EventReopened
			if input.Automatic {
				description = "Conversation reopened automatically"
			} else {
				description = fmt.Sprintf("Conversation reopened by %s", displayName(actorName, "Agent"))
			}
		}
	default:
		return nil
	}

	meta, _ := json.Marshal(map[string]any{
		"previous_status": string(input.Previous),
		"status":          string(input.Next),
		"automatic":       input.Automatic,
	})
	activity, err := s.recordActivity(ctx, recordActivityInput{
		TeamID:         input.TeamID,
		ConversationID: input.ConversationID,
		Type:           activityType,
		ActorID:        input.ActorID,
		Description:    description,
		Metadata:       meta,
	})
	if err != nil {
		return err
	}
	if eventType != "" {
		_ = s.publishActivity(ctx, input.TeamID, eventType, activity)
	}
	return nil
}

func (s *CollaborationService) RecordOpened(ctx context.Context, teamID, conversationID, actorID uuid.UUID) error {
	actorName := s.actorName(ctx, &actorID)
	activity, err := s.recordActivity(ctx, recordActivityInput{
		TeamID:         teamID,
		ConversationID: conversationID,
		Type:           ActivityOpened,
		ActorID:        &actorID,
		Description:    fmt.Sprintf("Conversation opened by %s", displayName(actorName, "Agent")),
	})
	if err != nil {
		return err
	}
	_ = activity
	return nil
}

type recordActivityInput struct {
	TeamID         uuid.UUID
	ConversationID uuid.UUID
	Type           ActivityType
	ActorID        *uuid.UUID
	Description    string
	Metadata       json.RawMessage
}

func (s *CollaborationService) recordActivity(ctx context.Context, input recordActivityInput) (*Activity, error) {
	if s.activities == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	activity := &Activity{
		ID:             uuid.New(),
		TeamID:         input.TeamID,
		ConversationID: input.ConversationID,
		Type:           input.Type,
		ActorID:        input.ActorID,
		ActorName:      s.actorName(ctx, input.ActorID),
		Description:    input.Description,
		Metadata:       input.Metadata,
		CreatedAt:      now,
	}
	if err := s.activities.Create(ctx, activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *CollaborationService) publishActivity(ctx context.Context, teamID uuid.UUID, eventType string, activity *Activity) error {
	if s.realtime == nil || activity == nil || eventType == "" {
		return nil
	}
	return s.realtime.PublishConversationEvent(ctx, teamID, eventType, activity)
}

func (s *CollaborationService) publishNote(ctx context.Context, teamID uuid.UUID, note *Note) error {
	if s.realtime == nil || note == nil {
		return nil
	}
	return s.realtime.PublishConversationEvent(ctx, teamID, EventNoteCreated, note)
}

func displayName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return name
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
