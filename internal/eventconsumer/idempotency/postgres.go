package idempotency

import (
	"context"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/idempotency/dbq"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// PostgresStore is the durable idempotency record (processed_events), shared by
// every replica and kept across restarts.
// Queries live in query.sql (sqlc, S5).
type PostgresStore struct {
	q *dbq.Queries
}

func NewPostgresStore(db *database.DB) *PostgresStore {
	return &PostgresStore{q: dbq.New(db.Pool)}
}

func (s *PostgresStore) Seen(ctx context.Context, key string) (bool, error) {
	return s.q.IsProcessed(ctx, key)
}

func (s *PostgresStore) Mark(ctx context.Context, key string) error {
	return s.q.MarkProcessed(ctx, key)
}

// PurgeOlderThan deletes records past the redelivery horizon and returns how many.
func (s *PostgresStore) PurgeOlderThan(ctx context.Context, age time.Duration) (int64, error) {
	return s.q.PurgeProcessedBefore(ctx, time.Now().Add(-age))
}

// CachedStore reads a fast cache (Redis) first and falls back to the durable
// store; Mark writes the durable store first. Cache failures never decide the
// outcome, so a Redis outage only costs a database round trip.
type CachedStore struct {
	durable Store
	cache   Store
}

func NewCachedStore(durable, cache Store) *CachedStore {
	return &CachedStore{durable: durable, cache: cache}
}

func (s *CachedStore) Seen(ctx context.Context, key string) (bool, error) {
	if s.cache != nil {
		if seen, err := s.cache.Seen(ctx, key); err == nil && seen {
			return true, nil
		}
	}
	seen, err := s.durable.Seen(ctx, key)
	if err == nil && seen && s.cache != nil {
		_ = s.cache.Mark(ctx, key) // warm the cache for the next redelivery
	}
	return seen, err
}

func (s *CachedStore) Mark(ctx context.Context, key string) error {
	if err := s.durable.Mark(ctx, key); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Mark(ctx, key)
	}
	return nil
}
