package inbox

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestCollisionService_OpenCloseMultipleAgents(t *testing.T) {
	store := NewMemoryCollisionStore()
	publisher := &recordingPublisher{}
	svc := NewCollisionService(store, publisher)
	teamID := uuid.New()
	convID := uuid.New()
	agentA := uuid.New()
	agentB := uuid.New()

	if _, err := svc.OpenConversation(context.Background(), teamID, convID, agentA, "Andi"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.OpenConversation(context.Background(), teamID, convID, agentB, "Budi")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Viewers) != 2 {
		t.Fatalf("expected 2 viewers, got %d", len(snapshot.Viewers))
	}
	if publisher.events[len(publisher.events)-1].Type != EventConversationOpened {
		t.Fatalf("expected opened event, got %s", publisher.events[len(publisher.events)-1].Type)
	}

	snapshot, err = svc.CloseConversation(context.Background(), teamID, convID, agentA, "Andi")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Viewers) != 1 {
		t.Fatalf("expected 1 viewer after close, got %d", len(snapshot.Viewers))
	}
	if publisher.events[len(publisher.events)-1].Type != EventConversationViewClosed {
		t.Fatalf("expected view closed event, got %s", publisher.events[len(publisher.events)-1].Type)
	}
}

func TestCollisionService_TypingLifecycle(t *testing.T) {
	store := NewMemoryCollisionStore()
	publisher := &recordingPublisher{}
	svc := NewCollisionService(store, publisher)
	teamID := uuid.New()
	convID := uuid.New()
	agentID := uuid.New()

	if _, err := svc.StartTyping(context.Background(), teamID, convID, agentID, "Andi"); err != nil {
		t.Fatal(err)
	}
	if publisher.events[len(publisher.events)-1].Type != EventAgentTypingStart {
		t.Fatalf("expected typing start event, got %s", publisher.events[len(publisher.events)-1].Type)
	}

	snapshot, err := svc.GetSnapshot(context.Background(), convID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Typing) != 1 || snapshot.Typing[0].AgentName != "Andi" {
		t.Fatalf("unexpected typing snapshot: %+v", snapshot.Typing)
	}

	if _, err := svc.StopTyping(context.Background(), teamID, convID, agentID, "Andi"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = svc.GetSnapshot(context.Background(), convID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Typing) != 0 {
		t.Fatalf("expected typing cleared, got %+v", snapshot.Typing)
	}
}

func TestCollisionService_ClearUserPresenceOnLogout(t *testing.T) {
	store := NewMemoryCollisionStore()
	publisher := &recordingPublisher{}
	svc := NewCollisionService(store, publisher)
	teamID := uuid.New()
	convID := uuid.New()
	agentID := uuid.New()

	if _, err := svc.OpenConversation(context.Background(), teamID, convID, agentID, "Andi"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartTyping(context.Background(), teamID, convID, agentID, "Andi"); err != nil {
		t.Fatal(err)
	}

	if err := svc.ClearUserPresence(context.Background(), teamID, agentID); err != nil {
		t.Fatal(err)
	}

	snapshot, err := svc.GetSnapshot(context.Background(), convID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Viewers) != 0 {
		t.Fatalf("expected viewers cleared after logout, got %+v", snapshot.Viewers)
	}
	if len(snapshot.Typing) != 0 {
		t.Fatalf("expected typing cleared after logout, got %+v", snapshot.Typing)
	}
	if publisher.events[len(publisher.events)-1].Type != EventConversationViewClosed {
		t.Fatalf("expected view closed event, got %s", publisher.events[len(publisher.events)-1].Type)
	}

	activeConv, _, err := store.GetActiveView(context.Background(), agentID.String())
	if err != nil {
		t.Fatal(err)
	}
	if activeConv != "" {
		t.Fatalf("expected active view cleared, got %q", activeConv)
	}
}

func TestCollisionService_OpenSwitchesConversation(t *testing.T) {
	store := NewMemoryCollisionStore()
	publisher := &recordingPublisher{}
	svc := NewCollisionService(store, publisher)
	teamID := uuid.New()
	convA := uuid.New()
	convB := uuid.New()
	agentID := uuid.New()

	if _, err := svc.OpenConversation(context.Background(), teamID, convA, agentID, "Andi"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenConversation(context.Background(), teamID, convB, agentID, "Andi"); err != nil {
		t.Fatal(err)
	}

	snapA, err := svc.GetSnapshot(context.Background(), convA)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapA.Viewers) != 0 {
		t.Fatalf("expected previous conversation cleared, got %+v", snapA.Viewers)
	}
	snapB, err := svc.GetSnapshot(context.Background(), convB)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapB.Viewers) != 1 {
		t.Fatalf("expected viewer on new conversation, got %+v", snapB.Viewers)
	}
}
