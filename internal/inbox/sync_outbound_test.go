package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

type recordingCollaboration struct {
	calls int
}

func (r *recordingCollaboration) RecordStatusChange(context.Context, conversation.StatusActivityInput) error {
	r.calls++
	return nil
}

func TestSyncService_HandleOutboundReopensClosedConversation(t *testing.T) {
	teamID := uuid.New()
	convID := uuid.New()
	conv := &conversation.Conversation{
		ID: convID, TeamID: teamID, Status: conversation.StatusClosed, UnreadCount: 0,
	}
	repo := &memConvRepo{conv: conv}
	rt := &memRT{}
	collab := &recordingCollaboration{}
	svc := inbox.NewSyncService(repo, nil, &memSettings{}, nil, rt)
	svc.SetCollaboration(collab)

	msg := &message.Message{ID: uuid.New(), Content: "reply", ReceivedAt: time.Now().UTC()}
	updated, err := svc.HandleOutbound(context.Background(), inbox.OutboundParams{
		TeamID: teamID, AgentID: uuid.New(), Conversation: conv, Message: msg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != conversation.StatusOpen {
		t.Fatalf("status=%s", updated.Status)
	}
	if collab.calls != 1 {
		t.Fatalf("expected reopen activity, calls=%d", collab.calls)
	}
}

// 9.2/9.3: an automated reply is not a human response: first response time, the last agent
// message and the customer's waiting clock are left untouched.
func TestSyncService_HandleOutboundAutomatedIsNotAHumanResponse(t *testing.T) {
	teamID, convID := uuid.New(), uuid.New()
	waiting := time.Now().Add(-time.Minute).UTC()
	conv := &conversation.Conversation{ID: convID, TeamID: teamID, Status: conversation.StatusOpen, WaitingSince: &waiting}
	repo := &memConvRepo{conv: conv}
	svc := inbox.NewSyncService(repo, nil, &memSettings{}, nil, &memRT{})

	msg := &message.Message{ID: uuid.New(), Content: "Selamat datang!"}
	if _, err := svc.HandleOutbound(context.Background(), inbox.OutboundParams{
		TeamID: teamID, AgentID: uuid.New(), Conversation: conv, Message: msg, Automated: true,
	}); err != nil {
		t.Fatal(err)
	}
	u := repo.syncs[len(repo.syncs)-1]
	if u.SetFirstResponseAt || u.LastAgentMessageAt != nil || u.ClearWaitingSince {
		t.Fatalf("automated reply touched human-response fields: %+v", u)
	}
	if !u.TouchLastMessage || u.LastMessagePreview != "Selamat datang!" {
		t.Fatalf("the conversation preview should still show the reply: %+v", u)
	}
}
