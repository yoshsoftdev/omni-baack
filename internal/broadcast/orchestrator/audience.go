package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// CampaignStore is the persistence surface for orchestration.
type CampaignStore interface {
	campaign.Repository
	campaign.OrchestrationRepository
}

// ContactLookup loads contacts for audience resolution.
type ContactLookup interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*contact.Contact, error)
	ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]contact.Contact, error)
}

// SegmentMembers lists segment membership.
type SegmentMembers interface {
	ListMemberContactIDs(ctx context.Context, segmentID uuid.UUID, page repository.PageParams) ([]uuid.UUID, int64, error)
}

// TagMembers lists contacts assigned to a tag.
type TagMembers interface {
	ListContactIDsByTag(ctx context.Context, teamID, tagID uuid.UUID) ([]uuid.UUID, error)
}

// AudienceResolver freezes audience and creates campaign_recipients.
type AudienceResolver struct {
	campaigns CampaignStore
	contacts  ContactLookup
	segments  SegmentMembers
	tags      TagMembers
	metrics   *Metrics
	log       *slog.Logger
	chunkSize int
}

func NewAudienceResolver(
	campaigns CampaignStore,
	contacts ContactLookup,
	segments SegmentMembers,
	tags TagMembers,
	metrics *Metrics,
	log *slog.Logger,
	chunkSize int,
) *AudienceResolver {
	if log == nil {
		log = slog.Default()
	}
	if chunkSize <= 0 {
		chunkSize = 5000
	}
	return &AudienceResolver{
		campaigns: campaigns,
		contacts:  contacts,
		segments:  segments,
		tags:      tags,
		metrics:   metrics,
		log:       log,
		chunkSize: chunkSize,
	}
}

type resolveResult struct {
	recipients     []campaign.Recipient
	resolved       int
	skippedOptOut  int
	skippedInvalid int
}

func (r *AudienceResolver) Resolve(ctx context.Context, job *campaign.BroadcastJob) error {
	count, err := r.campaigns.CountRecipientsByJob(ctx, job.ID)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC()
	job.Status = campaign.JobResolvingAudience
	job.UpdatedAt = now
	if err := r.campaigns.UpdateJob(ctx, job); err != nil {
		return err
	}

	camp, err := r.campaigns.FindByID(ctx, job.TeamID, job.CampaignID)
	if err != nil {
		return err
	}
	audiences, err := r.campaigns.ListAudiences(ctx, job.CampaignID)
	if err != nil {
		return err
	}

	result, err := r.collectRecipients(ctx, camp, job, audiences, now)
	if err != nil {
		return err
	}

	sourceSummary, _ := json.Marshal(map[string]any{
		"audiences":       audiences,
		"resolved_count":  result.resolved,
		"skipped_opt_out": result.skippedOptOut,
		"skipped_invalid": result.skippedInvalid,
		"snapshot_at":     now,
	})

	snap := &campaign.AudienceSnapshot{
		ID:              uuid.New(),
		CampaignID:      job.CampaignID,
		JobID:           job.ID,
		SnapshotAt:      now,
		SourceSummary:   sourceSummary,
		ResolvedCount:   result.resolved,
		SkippedOptOut:   result.skippedOptOut,
		SkippedInvalid:  result.skippedInvalid,
		CreatedAt:       now,
	}
	if err := r.campaigns.CreateAudienceSnapshot(ctx, snap); err != nil {
		return err
	}
	r.metrics.IncAudienceSnapshots()

	if len(result.recipients) > 0 {
		if err := r.campaigns.BulkCreateRecipients(ctx, result.recipients, now); err != nil {
			return err
		}
		r.metrics.AddRecipientsCreated(int64(len(result.recipients)))
	}

	if err := r.campaigns.UpdateCampaignTotalContacts(ctx, job.CampaignID, result.resolved); err != nil {
		return err
	}

	job.Status = campaign.JobPlanningBatches
	job.TotalRecipients = result.resolved + result.skippedOptOut + result.skippedInvalid
	job.UpdatedAt = time.Now().UTC()
	if err := r.campaigns.UpdateJob(ctx, job); err != nil {
		return err
	}

	r.log.Info("audience resolved",
		"campaign_id", job.CampaignID.String(),
		"job_id", job.ID.String(),
		"team_id", job.TeamID.String(),
		"channel_id", job.ChannelID.String(),
		"resolved", result.resolved,
		"skipped_opt_out", result.skippedOptOut,
		"skipped_invalid", result.skippedInvalid,
	)
	return nil
}

func (r *AudienceResolver) collectRecipients(
	ctx context.Context,
	camp *campaign.Campaign,
	job *campaign.BroadcastJob,
	audiences []campaign.Audience,
	now time.Time,
) (resolveResult, error) {
	seen := map[uuid.UUID]struct{}{}
	var contacts []contact.Contact

	for _, aud := range audiences {
		switch aud.SourceType {
		case campaign.AudienceContact:
			c, err := r.contacts.FindByID(ctx, camp.TeamID, aud.SourceID)
			if err != nil {
				if err == repository.ErrNotFound {
					continue
				}
				return resolveResult{}, err
			}
			if _, ok := seen[c.ID]; !ok {
				seen[c.ID] = struct{}{}
				contacts = append(contacts, *c)
			}
		case campaign.AudienceTag:
			ids, err := r.tags.ListContactIDsByTag(ctx, camp.TeamID, aud.SourceID)
			if err != nil {
				return resolveResult{}, err
			}
			batch, err := r.contacts.ListByIDs(ctx, camp.TeamID, ids)
			if err != nil {
				return resolveResult{}, err
			}
			for _, c := range batch {
				if _, ok := seen[c.ID]; !ok {
					seen[c.ID] = struct{}{}
					contacts = append(contacts, c)
				}
			}
		case campaign.AudienceSegment:
			// Legacy campaigns may still reference segments; skip resolving them.
			r.log.Warn("skipping legacy segment audience source",
				"campaign_id", camp.ID.String(),
				"source_id", aud.SourceID.String(),
			)
			continue
		}
	}

	var out resolveResult
	jobID := job.ID
	for _, c := range contacts {
		status, skipReason := classifyContact(c)
		rec := campaign.Recipient{
			ID:         uuid.New(),
			CampaignID: camp.ID,
			TeamID:     camp.TeamID,
			ContactID:  c.ID,
			JobID:      &jobID,
			Phone:      c.Phone,
			Status:     status,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		out.recipients = append(out.recipients, rec)
		switch skipReason {
		case skipOptOut:
			out.skippedOptOut++
		case skipInvalid:
			out.skippedInvalid++
		default:
			out.resolved++
		}
	}
	return out, nil
}

type skipReason int

const (
	skipNone skipReason = iota
	skipOptOut
	skipInvalid
)

func classifyContact(c contact.Contact) (campaign.RecipientStatus, skipReason) {
	if c.MarketingStatus == contact.MarketingUnsubscribed {
		return campaign.RecipientSkipped, skipOptOut
	}
	if !isWhatsAppEligibleContact(c) {
		return campaign.RecipientSkipped, skipInvalid
	}
	return campaign.RecipientPending, skipNone
}

// isWhatsAppEligibleContact rejects Instagram-only / synthetic phones that cannot receive WA templates.
func isWhatsAppEligibleContact(c contact.Contact) bool {
	if c.Source == contact.SourceInstagram {
		return false
	}
	return isValidWhatsAppPhone(c.Phone)
}

func isValidWhatsAppPhone(phone string) bool {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return false
	}
	lower := strings.ToLower(phone)
	if strings.HasPrefix(lower, "ig:") {
		return false
	}
	if len(phone) < 8 {
		return false
	}
	digits := 0
	for _, r := range phone {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return digits >= 8
}
