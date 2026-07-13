package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const (
	typingTTL         = 5 * time.Second
	viewersKeyPrefix  = "inbox:conv:"
	viewersKeySuffix  = ":viewers"
	typingKeyTemplate = "inbox:conv:%s:typing:%s"
)

// CollisionAgent is an agent participating in conversation presence.
type CollisionAgent struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
}

// CollisionSnapshot is the current viewers and typing agents for a conversation.
type CollisionSnapshot struct {
	ConversationID string           `json:"conversation_id"`
	AgentID        string           `json:"agent_id,omitempty"`
	AgentName      string           `json:"agent_name,omitempty"`
	Viewers        []CollisionAgent `json:"viewers"`
	Typing         []CollisionAgent `json:"typing"`
}

// CollisionStore persists ephemeral conversation presence.
type CollisionStore interface {
	AddViewer(ctx context.Context, conversationID, userID, agentName string) error
	RemoveViewer(ctx context.Context, conversationID, userID string) error
	ListViewers(ctx context.Context, conversationID string) ([]CollisionAgent, error)
	SetTyping(ctx context.Context, conversationID, userID, agentName string) error
	ClearTyping(ctx context.Context, conversationID, userID string) error
	ListTyping(ctx context.Context, conversationID string) ([]CollisionAgent, error)
}

// RedisCollisionStore stores viewers and typing state in Redis.
type RedisCollisionStore struct {
	client *goredis.Client
}

func NewRedisCollisionStore(client *goredis.Client) *RedisCollisionStore {
	return &RedisCollisionStore{client: client}
}

func viewersKey(conversationID string) string {
	return viewersKeyPrefix + conversationID + viewersKeySuffix
}

func typingKey(conversationID, userID string) string {
	return fmt.Sprintf(typingKeyTemplate, conversationID, userID)
}

func (s *RedisCollisionStore) AddViewer(ctx context.Context, conversationID, userID, agentName string) error {
	return s.client.HSet(ctx, viewersKey(conversationID), userID, agentName).Err()
}

func (s *RedisCollisionStore) RemoveViewer(ctx context.Context, conversationID, userID string) error {
	return s.client.HDel(ctx, viewersKey(conversationID), userID).Err()
}

func (s *RedisCollisionStore) ListViewers(ctx context.Context, conversationID string) ([]CollisionAgent, error) {
	values, err := s.client.HGetAll(ctx, viewersKey(conversationID)).Result()
	if err != nil {
		return nil, err
	}
	return mapAgents(values), nil
}

func (s *RedisCollisionStore) SetTyping(ctx context.Context, conversationID, userID, agentName string) error {
	return s.client.Set(ctx, typingKey(conversationID, userID), agentName, typingTTL).Err()
}

func (s *RedisCollisionStore) ClearTyping(ctx context.Context, conversationID, userID string) error {
	return s.client.Del(ctx, typingKey(conversationID, userID)).Err()
}

func (s *RedisCollisionStore) ListTyping(ctx context.Context, conversationID string) ([]CollisionAgent, error) {
	pattern := fmt.Sprintf("inbox:conv:%s:typing:*", conversationID)
	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()
	agents := make([]CollisionAgent, 0)
	for iter.Next(ctx) {
		key := iter.Val()
		userID := strings.TrimPrefix(key, fmt.Sprintf("inbox:conv:%s:typing:", conversationID))
		if userID == "" || userID == key {
			continue
		}
		name, err := s.client.Get(ctx, key).Result()
		if err == goredis.Nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		agents = append(agents, CollisionAgent{AgentID: userID, AgentName: name})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func mapAgents(values map[string]string) []CollisionAgent {
	agents := make([]CollisionAgent, 0, len(values))
	for id, name := range values {
		agents = append(agents, CollisionAgent{AgentID: id, AgentName: name})
	}
	return agents
}

// CollisionService coordinates conversation viewer and typing presence.
type CollisionService struct {
	store     CollisionStore
	publisher RealtimePublisher
}

func NewCollisionService(store CollisionStore, publisher RealtimePublisher) *CollisionService {
	return &CollisionService{store: store, publisher: publisher}
}

func (s *CollisionService) OpenConversation(ctx context.Context, teamID, conversationID, userID uuid.UUID, agentName string) (CollisionSnapshot, error) {
	convKey := conversationID.String()
	userKey := userID.String()
	if err := s.store.AddViewer(ctx, convKey, userKey, agentName); err != nil {
		return CollisionSnapshot{}, err
	}
	snapshot, err := s.snapshot(ctx, convKey, userKey, agentName)
	if err != nil {
		return CollisionSnapshot{}, err
	}
	if err := s.publish(ctx, teamID, EventConversationOpened, snapshot); err != nil {
		return CollisionSnapshot{}, err
	}
	return snapshot, nil
}

func (s *CollisionService) CloseConversation(ctx context.Context, teamID, conversationID, userID uuid.UUID, agentName string) (CollisionSnapshot, error) {
	convKey := conversationID.String()
	userKey := userID.String()
	if err := s.store.RemoveViewer(ctx, convKey, userKey); err != nil {
		return CollisionSnapshot{}, err
	}
	if err := s.store.ClearTyping(ctx, convKey, userKey); err != nil {
		return CollisionSnapshot{}, err
	}
	snapshot, err := s.snapshot(ctx, convKey, userKey, agentName)
	if err != nil {
		return CollisionSnapshot{}, err
	}
	if err := s.publish(ctx, teamID, EventConversationViewClosed, snapshot); err != nil {
		return CollisionSnapshot{}, err
	}
	return snapshot, nil
}

func (s *CollisionService) StartTyping(ctx context.Context, teamID, conversationID, userID uuid.UUID, agentName string) (CollisionSnapshot, error) {
	convKey := conversationID.String()
	userKey := userID.String()
	if err := s.store.SetTyping(ctx, convKey, userKey, agentName); err != nil {
		return CollisionSnapshot{}, err
	}
	snapshot, err := s.snapshot(ctx, convKey, userKey, agentName)
	if err != nil {
		return CollisionSnapshot{}, err
	}
	if err := s.publish(ctx, teamID, EventAgentTypingStart, snapshot); err != nil {
		return CollisionSnapshot{}, err
	}
	return snapshot, nil
}

func (s *CollisionService) StopTyping(ctx context.Context, teamID, conversationID, userID uuid.UUID, agentName string) (CollisionSnapshot, error) {
	convKey := conversationID.String()
	userKey := userID.String()
	if err := s.store.ClearTyping(ctx, convKey, userKey); err != nil {
		return CollisionSnapshot{}, err
	}
	snapshot, err := s.snapshot(ctx, convKey, userKey, agentName)
	if err != nil {
		return CollisionSnapshot{}, err
	}
	if err := s.publish(ctx, teamID, EventAgentTypingStop, snapshot); err != nil {
		return CollisionSnapshot{}, err
	}
	return snapshot, nil
}

func (s *CollisionService) GetSnapshot(ctx context.Context, conversationID uuid.UUID) (CollisionSnapshot, error) {
	return s.snapshot(ctx, conversationID.String(), "", "")
}

func (s *CollisionService) snapshot(ctx context.Context, conversationID, agentID, agentName string) (CollisionSnapshot, error) {
	viewers, err := s.store.ListViewers(ctx, conversationID)
	if err != nil {
		return CollisionSnapshot{}, err
	}
	typing, err := s.store.ListTyping(ctx, conversationID)
	if err != nil {
		return CollisionSnapshot{}, err
	}
	return CollisionSnapshot{
		ConversationID: conversationID,
		AgentID:        agentID,
		AgentName:      agentName,
		Viewers:        viewers,
		Typing:         typing,
	}, nil
}

func (s *CollisionService) publish(ctx context.Context, teamID uuid.UUID, eventType string, snapshot CollisionSnapshot) error {
	if s.publisher == nil {
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return s.publisher.PublishTeamEvent(ctx, teamID, RealtimeEvent{
		Type: eventType,
		Data: payload,
	})
}
