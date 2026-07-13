package inbox_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type memConvRepo struct {
	mu    sync.Mutex
	conv  *conversation.Conversation
	syncs []conversation.InboxSyncUpdate
}

func (m *memConvRepo) Create(context.Context, *conversation.Conversation) error { return nil }
func (m *memConvRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*conversation.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
func (m *memConvRepo) ListCursor(context.Context, uuid.UUID, conversation.ListFilter, *conversation.ListCursor, int) (conversation.CursorPage, error) {
	return conversation.CursorPage{}, nil
}
func (m *memConvRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, conversation.Status) error {
	return nil
}
func (m *memConvRepo) UpdateAssignment(_ context.Context, _, _ uuid.UUID, assignedTo *uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conv.AssignedTo = assignedTo
	return nil
}
func (m *memConvRepo) UpdateUnreadCount(context.Context, uuid.UUID, uuid.UUID, int) error { return nil }
func (m *memConvRepo) UpdateLastMessage(context.Context, uuid.UUID, uuid.UUID, conversation.LastMessageUpdate) error {
	return nil
}
func (m *memConvRepo) ApplyInboxSync(_ context.Context, _, _ uuid.UUID, u conversation.InboxSyncUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncs = append(m.syncs, u)
	if u.Status != "" {
		m.conv.Status = u.Status
	}
	m.conv.UnreadCount = u.UnreadCount
	if u.ClearResolutionAt {
		m.conv.ResolutionAt = nil
	}
	if u.ClearWaitingSince {
		m.conv.WaitingSince = nil
	}
	if u.WaitingSince != nil {
		m.conv.WaitingSince = u.WaitingSince
	}
	if u.BasePriority != nil {
		m.conv.BasePriority = *u.BasePriority
	}
	if u.PriorityScore != nil {
		m.conv.PriorityScore = *u.PriorityScore
	}
	return nil
}
func (m *memConvRepo) UpdatePriorityFields(_ context.Context, _, _ uuid.UUID, waitingSince *time.Time, clearWaiting bool, basePriority int, priorityScore int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if clearWaiting {
		m.conv.WaitingSince = nil
	} else if waitingSince != nil {
		m.conv.WaitingSince = waitingSince
	}
	m.conv.BasePriority = basePriority
	m.conv.PriorityScore = priorityScore
	return nil
}
func (m *memConvRepo) MarkRead(_ context.Context, _, _ uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conv.UnreadCount = 0
	return nil
}
func (m *memConvRepo) CountOpenByAssignee(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *memConvRepo) CountByTeamFilter(context.Context, uuid.UUID, conversation.ListFilter) (int64, error) {
	return 0, nil
}
func (m *memConvRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type memRT struct {
	events []inbox.RealtimeEvent
}

func (m *memRT) PublishTeamEvent(_ context.Context, teamID uuid.UUID, event inbox.RealtimeEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestSyncService_ReopenAndUnread(t *testing.T) {
	teamID := uuid.New()
	convID := uuid.New()
	conv := &conversation.Conversation{
		ID: convID, TeamID: teamID, Status: conversation.StatusClosed, UnreadCount: 0,
	}
	repo := &memConvRepo{conv: conv}
	rt := &memRT{}
	svc := inbox.NewSyncService(repo, nil, &memSettings{}, nil, rt)

	msg := &message.Message{ID: uuid.New(), Content: "hi", ReceivedAt: time.Now().UTC()}
	_, err := svc.HandleInbound(context.Background(), inbox.InboundParams{
		TeamID: teamID, Conversation: conv, Message: msg, MessageCreated: true, Preview: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.conv.Status != conversation.StatusOpen {
		t.Fatalf("status %s", repo.conv.Status)
	}
	if len(repo.syncs) == 0 || repo.syncs[0].UnreadCount != 1 {
		t.Fatalf("unread not incremented: %+v", repo.syncs)
	}
	if len(rt.events) == 0 {
		t.Fatal("expected realtime event")
	}
}

func TestSyncService_MarkRead(t *testing.T) {
	teamID := uuid.New()
	conv := &conversation.Conversation{ID: uuid.New(), TeamID: teamID, UnreadCount: 3}
	repo := &memConvRepo{conv: conv}
	rt := &memRT{}
	svc := inbox.NewSyncService(repo, nil, &memSettings{}, nil, rt)
	if _, err := svc.MarkRead(context.Background(), teamID, conv.ID); err != nil {
		t.Fatal(err)
	}
	if conv.UnreadCount != 0 {
		t.Fatalf("unread=%d", conv.UnreadCount)
	}
}
