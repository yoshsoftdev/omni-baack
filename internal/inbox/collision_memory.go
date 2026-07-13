package inbox

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// MemoryCollisionStore is an in-memory CollisionStore for tests.
type MemoryCollisionStore struct {
	mu      sync.Mutex
	viewers map[string]map[string]string
	typing  map[string]map[string]string
}

func NewMemoryCollisionStore() *MemoryCollisionStore {
	return &MemoryCollisionStore{
		viewers: make(map[string]map[string]string),
		typing:  make(map[string]map[string]string),
	}
}

func (s *MemoryCollisionStore) AddViewer(_ context.Context, conversationID, userID, agentName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.viewers[conversationID] == nil {
		s.viewers[conversationID] = make(map[string]string)
	}
	s.viewers[conversationID][userID] = agentName
	return nil
}

func (s *MemoryCollisionStore) RemoveViewer(_ context.Context, conversationID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.viewers[conversationID], userID)
	return nil
}

func (s *MemoryCollisionStore) ListViewers(_ context.Context, conversationID string) ([]CollisionAgent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return mapAgents(s.viewers[conversationID]), nil
}

func (s *MemoryCollisionStore) SetTyping(_ context.Context, conversationID, userID, agentName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.typing[conversationID] == nil {
		s.typing[conversationID] = make(map[string]string)
	}
	s.typing[conversationID][userID] = agentName
	return nil
}

func (s *MemoryCollisionStore) ClearTyping(_ context.Context, conversationID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.typing[conversationID], userID)
	return nil
}

func (s *MemoryCollisionStore) ListTyping(_ context.Context, conversationID string) ([]CollisionAgent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return mapAgents(s.typing[conversationID]), nil
}

type recordingPublisher struct {
	events []RealtimeEvent
}

func (p *recordingPublisher) PublishTeamEvent(_ context.Context, _ uuid.UUID, event RealtimeEvent) error {
	p.events = append(p.events, event)
	return nil
}
