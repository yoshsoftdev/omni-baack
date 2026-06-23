package campaign

import (
	"context"

	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

// Scheduler is a no-op placeholder until scheduled campaign dispatch is migrated.
type Scheduler struct{}

// NewScheduler returns a scheduler stub that satisfies bootstrap wiring.
func NewScheduler(_ *Service, _ *logger.Logger) *Scheduler {
	return &Scheduler{}
}

// Start blocks until the context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	<-ctx.Done()
}
