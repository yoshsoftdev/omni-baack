package inbox_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
)

func TestToDelta_SlimPayload(t *testing.T) {
	now := time.Now().UTC()
	convID := uuid.New()
	item := conversation.Item{
		Conversation: conversation.Conversation{
			ID:                 convID,
			Status:             conversation.StatusOpen,
			UnreadCount:        2,
			LastMessagePreview: "hello",
			LastMessageAt:      &now,
			PriorityScore:      120,
			WaitingSince:       &now,
		},
		ContactName:  "Alice",
		ContactPhone: "+123",
	}
	delta := inbox.ToDelta(item)
	raw, err := json.Marshal(delta)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["team_id"]; ok {
		t.Fatal("delta should not include team_id")
	}
	if payload["conversation_id"] != convID.String() {
		t.Fatalf("conversation_id=%v", payload["conversation_id"])
	}
	if payload["priority_score"].(float64) != 120 {
		t.Fatalf("priority_score=%v", payload["priority_score"])
	}
}

func TestMarkRead_PublishesUnreadChanged(t *testing.T) {
	teamID := uuid.New()
	conv := &conversation.Conversation{ID: uuid.New(), TeamID: teamID, UnreadCount: 3}
	repo := &memConvRepo{conv: conv}
	rt := &memRT{}
	svc := inbox.NewSyncService(repo, nil, &memSettings{}, nil, rt)
	if _, err := svc.MarkRead(t.Context(), teamID, conv.ID); err != nil {
		t.Fatal(err)
	}
	if len(rt.events) == 0 {
		t.Fatal("expected event")
	}
	if rt.events[len(rt.events)-1].Type != inbox.EventConversationUnreadChanged {
		t.Fatalf("type=%s", rt.events[len(rt.events)-1].Type)
	}
}
