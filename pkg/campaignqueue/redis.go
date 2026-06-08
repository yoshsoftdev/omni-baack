package campaignqueue

import (
	"context"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

type RedisQueue struct {
	client *goredis.Client
}

func NewRedisQueue(client *goredis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

func (q *RedisQueue) Enqueue(ctx context.Context, campaignID uuid.UUID) error {
	return q.client.LPush(ctx, RedisKey, campaignID.String()).Err()
}

func (q *RedisQueue) Dequeue(ctx context.Context) (uuid.UUID, error) {
	result, err := q.client.BRPop(ctx, 2*time.Second, RedisKey).Result()
	if err != nil {
		if err == goredis.Nil {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}
	if len(result) < 2 {
		return uuid.Nil, nil
	}
	return uuid.Parse(result[1])
}
