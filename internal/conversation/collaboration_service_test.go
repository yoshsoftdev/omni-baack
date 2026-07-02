package conversation_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type memActivityRepo struct {
	mu    sync.Mutex
	items []conversation.Activity
}

func (m *memActivityRepo) Create(_ context.Context, activity *conversation.Activity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, *activity)
	return nil
}

func (m *memActivityRepo) ListByConversation(_ context.Context, _, _ uuid.UUID, _ int) ([]conversation.Activity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]conversation.Activity, len(m.items))
	copy(out, m.items)
	return out, nil
}

type memConvRepo struct {
	conv *conversation.Conversation
}

func (m *memConvRepo) Create(context.Context, *conversation.Conversation) error { return nil }
func (m *memConvRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*conversation.Conversation, error) {
	if m.conv == nil {
		return nil, repository.ErrNotFound
	}
	cp := *m.conv
	return &cp, nil
}
func (m *memConvRepo) FindByContactChannel(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (*conversation.Conversation, error) {
	return nil, repository.ErrNotFound
}
func (m *memConvRepo) List(context.Context, uuid.UUID, conversation.ListFilter, repository.PageParams) (repository.Page[conversation.Conversation], error) {
	return repository.Page[conversation.Conversation]{}, nil
}
func (m *memConvRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, conversation.Status) error { return nil }
func (m *memConvRepo) UpdateAssignment(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error     { return nil }
func (m *memConvRepo) UpdateUnreadCount(context.Context, uuid.UUID, uuid.UUID, int) error            { return nil }
func (m *memConvRepo) UpdateLastMessage(context.Context, uuid.UUID, uuid.UUID, conversation.LastMessageUpdate) error {
	return nil
}
func (m *memConvRepo) ApplyInboxSync(context.Context, uuid.UUID, uuid.UUID, conversation.InboxSyncUpdate) error {
	return nil
}
func (m *memConvRepo) MarkRead(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memConvRepo) CountOpenByAssignee(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *memConvRepo) CountByTeamFilter(context.Context, uuid.UUID, conversation.ListFilter) (int64, error) {
	return 0, nil
}
func (m *memConvRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type stubUserRepo struct {
	users map[uuid.UUID]*user.User
}

func (s *stubUserRepo) Create(context.Context, *user.User) error { return nil }
func (s *stubUserRepo) FindByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}
func (s *stubUserRepo) FindByEmail(context.Context, string) (*user.User, error) { return nil, repository.ErrNotFound }
func (s *stubUserRepo) Update(context.Context, *user.User) error                { return nil }
func (s *stubUserRepo) UpdateLastActive(context.Context, uuid.UUID, time.Time) error {
	return nil
}
func (s *stubUserRepo) SoftDelete(context.Context, uuid.UUID) error { return nil }
func (s *stubUserRepo) CreateRefreshToken(context.Context, *user.RefreshToken) error {
	return nil
}
func (s *stubUserRepo) FindRefreshTokenByHash(context.Context, string) (*user.RefreshToken, error) {
	return nil, repository.ErrNotFound
}
func (s *stubUserRepo) RevokeRefreshToken(context.Context, uuid.UUID, time.Time) error { return nil }

type memRT struct {
	events []struct {
		Type string
	}
}

func (m *memRT) PublishConversationEvent(_ context.Context, _ uuid.UUID, eventType string, _ any) error {
	m.events = append(m.events, struct{ Type string }{Type: eventType})
	return nil
}

func testRBAC() *rbac.Service {
	return rbac.NewService(nil)
}

func TestCollaborationService_RecordAssignment(t *testing.T) {
	teamID := uuid.New()
	convID := uuid.New()
	actorID := uuid.New()
	assigneeID := uuid.New()
	activities := &memActivityRepo{}
	rt := &memRT{}
	svc := conversation.NewCollaborationService(
		activities,
		nil,
		&memConvRepo{conv: &conversation.Conversation{ID: convID, TeamID: teamID}},
		nil,
		&stubUserRepo{users: map[uuid.UUID]*user.User{
			actorID:    {ID: actorID, Name: "Admin"},
			assigneeID: {ID: assigneeID, Name: "Steven"},
		}},
		testRBAC(),
		rt,
	)

	if err := svc.RecordAssignment(context.Background(), conversation.AssignmentActivityInput{
		TeamID: teamID, ConversationID: convID, ActorID: actorID, Assignee: &assigneeID,
	}); err != nil {
		t.Fatal(err)
	}
	if len(activities.items) != 1 || activities.items[0].Type != conversation.ActivityAssigned {
		t.Fatalf("activity=%+v", activities.items)
	}
	if len(rt.events) == 0 || rt.events[0].Type != conversation.EventAssigned {
		t.Fatalf("events=%+v", rt.events)
	}
}

func TestCollaborationService_CreateNoteForbidden(t *testing.T) {
	teamID := uuid.New()
	convID := uuid.New()
	userID := uuid.New()
	svc := conversation.NewCollaborationService(
		&memActivityRepo{},
		nil,
		&memConvRepo{conv: &conversation.Conversation{ID: convID, TeamID: teamID}},
		nil,
		&stubUserRepo{users: map[uuid.UUID]*user.User{userID: {ID: userID, Name: "Steven"}}},
		testRBAC(),
		nil,
	)
	_, err := svc.CreateNote(context.Background(), teamID, userID, "agent", convID, conversation.CreateNoteInput{Content: "note"})
	if err == nil {
		t.Fatal("expected forbidden without conversation.note permission")
	}
}

func TestCollaborationService_AutomaticReopenActivity(t *testing.T) {
	teamID := uuid.New()
	convID := uuid.New()
	activities := &memActivityRepo{}
	rt := &memRT{}
	svc := conversation.NewCollaborationService(
		activities,
		nil,
		&memConvRepo{conv: &conversation.Conversation{ID: convID, TeamID: teamID, Status: conversation.StatusOpen}},
		nil,
		&stubUserRepo{},
		testRBAC(),
		rt,
	)
	if err := svc.RecordStatusChange(context.Background(), conversation.StatusActivityInput{
		TeamID: teamID, ConversationID: convID, Previous: conversation.StatusClosed, Next: conversation.StatusOpen, Automatic: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(activities.items) != 1 || activities.items[0].Description != "Conversation reopened automatically" {
		t.Fatalf("activity=%+v", activities.items)
	}
}
