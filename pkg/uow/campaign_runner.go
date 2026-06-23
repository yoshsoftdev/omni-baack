package uow

import (
	"context"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// CampaignRunner adapts TransactionManager to campaign.UnitOfWorkRunner.
type CampaignRunner struct {
	manager TransactionManager
}

// NewCampaignRunner returns a campaign.UnitOfWorkRunner backed by the database pool.
func NewCampaignRunner(db *database.DB) campaign.UnitOfWorkRunner {
	return &CampaignRunner{manager: NewManager(db, NewFactory())}
}

// NewCampaignRunnerWithManager wires an existing TransactionManager (for tests).
func NewCampaignRunnerWithManager(m TransactionManager) campaign.UnitOfWorkRunner {
	return &CampaignRunner{manager: m}
}

func (r *CampaignRunner) Run(ctx context.Context, fn func(ctx context.Context, repos campaign.Repository) error) error {
	return r.manager.Run(ctx, func(ctx context.Context, work UnitOfWork) error {
		return fn(ctx, work.Campaigns())
	})
}
