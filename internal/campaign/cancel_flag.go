package campaign

import (
	"context"

	"github.com/google/uuid"
)

// CancelFlagStore sets fast cancel signals for horizontally scaled send workers.
type CancelFlagStore interface {
	Set(ctx context.Context, campaignID uuid.UUID) error
	IsSet(ctx context.Context, campaignID uuid.UUID) (bool, error)
}
