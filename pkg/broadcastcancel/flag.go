package broadcastcancel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const (
	// KeyPrefix is the Redis key prefix for campaign cancel flags.
	KeyPrefix = "cancel:campaign:"
	// DefaultTTL is how long a cancel flag remains visible to workers.
	DefaultTTL = 24 * time.Hour
)

// Store reads and writes campaign cancel flags in Redis.
type Store struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewStore(client *goredis.Client, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{client: client, ttl: ttl}
}

func key(campaignID uuid.UUID) string {
	return KeyPrefix + campaignID.String()
}

// Set marks a campaign as cancelled for fast worker polling.
func (s *Store) Set(ctx context.Context, campaignID uuid.UUID) error {
	return s.client.Set(ctx, key(campaignID), "1", s.ttl).Err()
}

// IsSet reports whether the cancel flag is present.
func (s *Store) IsSet(ctx context.Context, campaignID uuid.UUID) (bool, error) {
	n, err := s.client.Exists(ctx, key(campaignID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// KeyFor returns the Redis key for a campaign (for tests and ops).
func KeyFor(campaignID uuid.UUID) string {
	return key(campaignID)
}

// ErrRedisUnavailable indicates Redis could not be reached.
var ErrRedisUnavailable = fmt.Errorf("redis unavailable")
