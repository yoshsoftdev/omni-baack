package hub

import (
	"context"
	"encoding/json"
	"sync"

	goredis "github.com/redis/go-redis/v9"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

const redisChannelPrefix = "ws:tenant:"

// Hub manages WebSocket clients grouped by tenant. Redis pub/sub can fan out
// events across instances (skeleton — subscribe/publish hooks ready for Phase 1).
type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]map[*Client]struct{}
	log     *logger.Logger
	redis   *goredis.Client
	onClose func(*Client)
}

func New(log *logger.Logger, redis *goredis.Client) *Hub {
	return &Hub{
		rooms: make(map[string]map[*Client]struct{}),
		log:   log,
		redis: redis,
	}
}

func (h *Hub) Register(client *Client) {
	room := tenantRoom(client.TenantID)
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*Client]struct{})
	}
	h.rooms[room][client] = struct{}{}
}

func (h *Hub) Unregister(client *Client) {
	room := tenantRoom(client.TenantID)
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.rooms[room]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.rooms, room)
		}
	}
	client.Close()
	if h.onClose != nil {
		h.onClose(client)
	}
}

func (h *Hub) BroadcastToTenant(tenantID string, payload []byte) {
	room := tenantRoom(tenantID)
	h.mu.RLock()
	clients := h.rooms[room]
	h.mu.RUnlock()

	for client := range clients {
		select {
		case client.send <- payload:
		default:
			h.log.Warn("client send buffer full, dropping message", "tenant_id", tenantID)
		}
	}
}

// PublishTenant publishes an event to Redis for cross-instance delivery.
func (h *Hub) PublishTenant(ctx context.Context, tenantID string, event any) error {
	if h.redis == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	channel := redisChannelPrefix + tenantID
	return h.redis.Publish(ctx, channel, data).Err()
}

// StartRedisSubscriber listens for tenant events (no-op when redis is nil).
func (h *Hub) StartRedisSubscriber(ctx context.Context) error {
	if h.redis == nil {
		return nil
	}

	pubsub := h.redis.PSubscribe(ctx, redisChannelPrefix+"*")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			tenantID := tenantIDFromChannel(msg.Channel)
			h.BroadcastToTenant(tenantID, []byte(msg.Payload))
		}
	}
}

func tenantRoom(tenantID string) string {
	return "tenant:" + tenantID
}

func tenantIDFromChannel(channel string) string {
	const prefix = redisChannelPrefix
	if len(channel) > len(prefix) {
		return channel[len(prefix):]
	}
	return channel
}
