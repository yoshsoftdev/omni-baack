package idempotency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Store suppresses duplicate consumer processing.
type Store interface {
	Seen(ctx context.Context, key string) (bool, error)
	Mark(ctx context.Context, key string) error
}

type memoryEntry struct {
	expiresAt time.Time
}

// MemoryStore is an in-process TTL cache for development/tests.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]memoryEntry
	ttl     time.Duration
}

func NewMemoryStore(ttl time.Duration) *MemoryStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &MemoryStore{entries: make(map[string]memoryEntry), ttl: ttl}
}

func (s *MemoryStore) Seen(ctx context.Context, key string) (bool, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	_, ok := s.entries[key]
	return ok, nil
}

func (s *MemoryStore) Mark(ctx context.Context, key string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = memoryEntry{expiresAt: time.Now().Add(s.ttl)}
	return nil
}

func (s *MemoryStore) purgeLocked() {
	now := time.Now()
	for k, v := range s.entries {
		if now.After(v.expiresAt) {
			delete(s.entries, k)
		}
	}
}

// Key builders for consumer idempotency.
func MessageKey(provider, wamid string) string {
	return provider + ":message:" + wamid
}

func StatusKey(provider, wamid, status string) string {
	return provider + ":status:" + wamid + ":" + status
}

func TemplateKey(provider, templateID string, updatedAt int64) string {
	return fmt.Sprintf("%s:template:%s:%d", provider, templateID, updatedAt)
}

func ChannelKey(provider, field, phoneNumberID string, updatedAt int64) string {
	return fmt.Sprintf("%s:channel:%s:%s:%d", provider, field, phoneNumberID, updatedAt)
}
