package broadcast

import (
	"context"
	"time"

	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

type Scheduler struct {
	service  *Service
	log      *logger.Logger
	interval time.Duration
}

func NewScheduler(service *Service, log *logger.Logger) *Scheduler {
	return &Scheduler{
		service:  service,
		log:      log,
		interval: 30 * time.Second,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.service.ProcessDue(ctx); err != nil {
				s.log.Error("broadcast scheduler failed", "error", err)
			}
		}
	}
}
