package workerregistry

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	RoleOrchestrator  = "orchestrator"
	RoleSendWorker    = "send_worker"
	RoleEventConsumer = "event_consumer"

	keyPrefix        = "ops:worker:"
	defaultTTL       = 30 * time.Second
	defaultHeartbeat = 10 * time.Second
)

// Registry registers a worker instance for operational visibility.
type Registry struct {
	client *goredis.Client
	role   string
	id     string
	ttl    time.Duration
}

func New(client *goredis.Client, role, id string) *Registry {
	return &Registry{
		client: client,
		role:   role,
		id:     id,
		ttl:    defaultTTL,
	}
}

func (r *Registry) key() string {
	return fmt.Sprintf("%s%s:%s", keyPrefix, r.role, r.id)
}

// Register writes a heartbeat key with TTL.
func (r *Registry) Register(ctx context.Context) error {
	return r.client.Set(ctx, r.key(), time.Now().UTC().Format(time.RFC3339Nano), r.ttl).Err()
}

// StartHeartbeat refreshes the registration until context cancellation.
func (r *Registry) StartHeartbeat(ctx context.Context) {
	interval := defaultHeartbeat
	if r.ttl/3 < interval {
		interval = r.ttl / 3
	}
	if interval <= 0 {
		interval = defaultHeartbeat
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	_ = r.Register(ctx)
	for {
		select {
		case <-ctx.Done():
			_ = r.client.Del(context.Background(), r.key()).Err()
			return
		case <-ticker.C:
			_ = r.Register(ctx)
		}
	}
}

// CountActive returns the number of registered workers for a role.
func CountActive(ctx context.Context, client *goredis.Client, role string) (int, error) {
	pattern := fmt.Sprintf("%s%s:*", keyPrefix, role)
	var cursor uint64
	total := 0
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return 0, err
		}
		total += len(keys)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return total, nil
}
