package inbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
)

// HubPublisher publishes realtime events via the WebSocket hub + Redis fanout.
type HubPublisher struct {
	hub *hub.Hub
}

func NewHubPublisher(h *hub.Hub) *HubPublisher {
	return &HubPublisher{hub: h}
}

func (p *HubPublisher) PublishTeamEvent(ctx context.Context, teamID uuid.UUID, event RealtimeEvent) error {
	if p.hub == nil {
		return nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.TeamID == "" {
		event.TeamID = teamID.String()
	}
	return p.hub.PublishTenant(ctx, teamID.String(), event)
}

// BroadcastLocal delivers an event to clients connected to this instance.
func (p *HubPublisher) BroadcastLocal(teamID uuid.UUID, event RealtimeEvent) {
	if p.hub == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	p.hub.BroadcastToTenant(teamID.String(), data)
}
