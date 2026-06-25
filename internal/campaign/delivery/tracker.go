package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Tracker correlates Meta status webhooks to campaign recipients.
type Tracker struct {
	campaigns  campaign.Repository
	aggregator *ReportAggregator
	completion *CompletionMonitor
	metrics    *Metrics
	log        *slog.Logger
}

func NewTracker(
	campaigns campaign.Repository,
	aggregator *ReportAggregator,
	completion *CompletionMonitor,
	metrics *Metrics,
	log *slog.Logger,
) *Tracker {
	if log == nil {
		log = slog.Default()
	}
	if metrics == nil {
		metrics = &Metrics{}
	}
	return &Tracker{
		campaigns:  campaigns,
		aggregator: aggregator,
		completion: completion,
		metrics:    metrics,
		log:        log,
	}
}

// HandleStatus updates campaign recipient state from a canonical status event.
func (t *Tracker) HandleStatus(
	ctx context.Context,
	teamID uuid.UUID,
	eventType string,
	wamid string,
	failureText string,
	rawPayload []byte,
	at time.Time,
) error {
	target := RecipientStatusFromEvent(eventType)
	if target == "" || wamid == "" {
		return nil
	}

	rec, err := t.campaigns.FindRecipientByWAMIDForTeam(ctx, teamID, wamid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	if rec.TeamID != teamID {
		return nil
	}

	from := rec.Status
	if from == target {
		return nil
	}
	if !CanTransition(from, target) {
		return nil
	}

	update := campaign.RecipientDeliveryUpdate{WAMID: &wamid}
	if target == campaign.RecipientFailed {
		code, reason := ClassifyFailure(extractFailureText(failureText, rawPayload))
		update.LastError = &reason
		update.FailureCode = &code
		t.metrics.IncRecipientFailures()
	}

	if err := t.campaigns.UpdateRecipientDelivery(ctx, rec.ID, from, target, update); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}

	if err := t.aggregator.ApplyTransition(ctx, rec, from, target, at); err != nil {
		return err
	}
	if CounterDeltaForTransition(from, target) != (campaign.CounterDelta{}) {
		t.metrics.IncCounterUpdates()
	}
	t.metrics.IncStatusUpdates()

	t.log.Info("campaign recipient status updated",
		"campaign_id", rec.CampaignID.String(),
		"recipient_id", rec.ID.String(),
		"wamid", wamid,
		"status", string(target),
	)

	if t.completion != nil {
		if _, err := t.completion.Check(ctx, rec.TeamID, rec.CampaignID, at); err != nil {
			return err
		}
	}
	return nil
}

func extractFailureText(failureText string, raw []byte) string {
	if strings.TrimSpace(failureText) != "" {
		return failureText
	}
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Statuses []struct {
			Errors []struct {
				Title   string `json:"title"`
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	for _, st := range payload.Statuses {
		for _, e := range st.Errors {
			if e.Message != "" {
				return e.Message
			}
			if e.Title != "" {
				return e.Title
			}
		}
	}
	return ""
}
