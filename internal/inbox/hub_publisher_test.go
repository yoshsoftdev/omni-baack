package inbox_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

func TestHubPublisher_BroadcastLocal(t *testing.T) {
	log := logger.New("test")
	h := hub.New(log, nil)
	pub := inbox.NewHubPublisher(h)
	teamID := uuid.New()
	event := inbox.RealtimeEvent{Type: inbox.EventConversationUpdated, TeamID: teamID.String()}
	pub.BroadcastLocal(teamID, event)
	// No clients registered — smoke test only.
	_ = pub.PublishTeamEvent(context.Background(), teamID, event)
	data, _ := json.Marshal(event)
	if len(data) == 0 {
		t.Fatal("expected event payload")
	}
}
