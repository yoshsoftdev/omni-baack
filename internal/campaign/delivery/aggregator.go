package delivery

import (
	"context"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/report"
)

// ReportAggregator updates campaign counters and daily stats idempotently per transition.
type ReportAggregator struct {
	campaigns campaign.Repository
	reports   report.Repository
}

func NewReportAggregator(campaigns campaign.Repository, reports report.Repository) *ReportAggregator {
	return &ReportAggregator{campaigns: campaigns, reports: reports}
}

// ApplyTransition increments counters and daily stats when delta is non-zero.
func (a *ReportAggregator) ApplyTransition(ctx context.Context, rec *campaign.Recipient, from, to campaign.RecipientStatus, at time.Time) error {
	delta := CounterDeltaForTransition(from, to)
	if delta.Sent == 0 && delta.Delivered == 0 && delta.Read == 0 && delta.Failed == 0 {
		return nil
	}
	if err := a.campaigns.ApplyCounterDelta(ctx, rec.CampaignID, delta); err != nil {
		return err
	}
	statDate := at.UTC().Truncate(24 * time.Hour)
	return a.reports.IncrementDailyStat(ctx, rec.TeamID, statDate, report.DailyStatDelta{
		MessagesSent:      delta.Sent,
		MessagesDelivered: delta.Delivered,
		MessagesRead:      delta.Read,
		MessagesFailed:    delta.Failed,
	})
}
