//go:build legacy

package campaign

import (
	"context"
	"time"

	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

type Scheduler struct {
	service *Service
	log     *logger.Logger
}

func NewScheduler(service *Service, log *logger.Logger) *Scheduler {
	return &Scheduler{service: service, log: log}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now().UTC()
	due, err := s.service.campaigns.ListDueScheduled(ctx, now, 50)
	if err != nil {
		s.log.Error("list due campaigns failed", "error", err)
		return
	}
	for i := range due {
		campaign := &due[i]
		tagIDs, err := s.service.campaigns.ListTagIDs(ctx, campaign.ID)
		if err != nil {
			s.log.Error("list campaign tags failed", "campaign_id", campaign.ID, "error", err)
			continue
		}
		if err := s.service.prepareAndQueue(ctx, campaign, nil, tagIDs); err != nil {
			s.log.Error("queue scheduled campaign failed", "campaign_id", campaign.ID, "error", err)
		}
	}
}
