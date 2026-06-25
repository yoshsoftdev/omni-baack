package delivery

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// CompletionMonitor marks campaigns completed when all recipients are terminal.
type CompletionMonitor struct {
	campaigns campaign.Repository
	metrics   *Metrics
	log       *slog.Logger
}

func NewCompletionMonitor(campaigns campaign.Repository, metrics *Metrics, log *slog.Logger) *CompletionMonitor {
	if log == nil {
		log = slog.Default()
	}
	if metrics == nil {
		metrics = &Metrics{}
	}
	return &CompletionMonitor{campaigns: campaigns, metrics: metrics, log: log}
}

// Check completes the campaign when no non-terminal recipients remain.
func (m *CompletionMonitor) Check(ctx context.Context, teamID, campaignID uuid.UUID, at time.Time) (bool, error) {
	remaining, err := m.campaigns.CountNonTerminalRecipients(ctx, campaignID)
	if err != nil {
		return false, err
	}
	if remaining > 0 {
		return false, nil
	}

	err = m.campaigns.CompleteCampaign(ctx, teamID, campaignID, at.UTC())
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	m.metrics.IncCompletion()
	m.log.Info("campaign completed",
		"campaign_id", campaignID.String(),
		"team_id", teamID.String(),
	)
	return true, nil
}
