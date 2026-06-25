package operations

import (
	"context"

	"github.com/omnichannel/omni-chan-v2/pkg/workerregistry"
	goredis "github.com/redis/go-redis/v9"
)

// RedisWorkerCounter counts worker heartbeats in Redis.
type RedisWorkerCounter struct {
	client *goredis.Client
}

func NewRedisWorkerCounter(client *goredis.Client) *RedisWorkerCounter {
	return &RedisWorkerCounter{client: client}
}

func (c *RedisWorkerCounter) CountOrchestrators(ctx context.Context) (int, error) {
	return workerregistry.CountActive(ctx, c.client, workerregistry.RoleOrchestrator)
}

func (c *RedisWorkerCounter) CountSendWorkers(ctx context.Context) (int, error) {
	return workerregistry.CountActive(ctx, c.client, workerregistry.RoleSendWorker)
}

func (c *RedisWorkerCounter) CountEventConsumers(ctx context.Context) (int, error) {
	return workerregistry.CountActive(ctx, c.client, workerregistry.RoleEventConsumer)
}
