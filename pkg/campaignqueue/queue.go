package campaignqueue

import (
	"context"

	"github.com/google/uuid"
)

const RedisKey = "omni:campaign:send"

type Queue interface {
	Enqueue(ctx context.Context, campaignID uuid.UUID) error
	Dequeue(ctx context.Context) (uuid.UUID, error)
}
