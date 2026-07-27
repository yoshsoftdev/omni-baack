package campaign

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/segment"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (CampaignService) manages broadcast campaigns and draft execution artifacts.
type Service struct {
	campaigns   Repository
	tx          UnitOfWorkRunner
	teams       team.Repository
	channels    channel.Repository
	templates   template.Repository
	segments    segment.Repository
	rbac        *rbac.Service
	cancelFlags CancelFlagStore
	payment     CampaignPaymentChecker
	snapshots   *recipientSnapshotDeps
}

type CampaignPaymentChecker interface {
	ChannelPaymentConfigured(ctx context.Context, teamID, channelID uuid.UUID) (bool, error)
}

// ServiceOption configures optional campaign service dependencies.
type ServiceOption func(*Service)

// WithCancelFlags enables Redis cancel flag propagation on campaign cancel.
func WithCancelFlags(store CancelFlagStore) ServiceOption {
	return func(s *Service) {
		s.cancelFlags = store
	}
}

// WithPaymentChecker enforces Meta payment configuration before campaign send.
func WithPaymentChecker(checker CampaignPaymentChecker) ServiceOption {
	return func(s *Service) {
		s.payment = checker
	}
}

func NewService(
	campaigns Repository,
	tx UnitOfWorkRunner,
	teams team.Repository,
	channels channel.Repository,
	templates template.Repository,
	segments segment.Repository,
	rbacSvc *rbac.Service,
	opts ...ServiceOption,
) *Service {
	s := &Service{
		campaigns: campaigns,
		tx:        tx,
		teams:     teams,
		channels:  channels,
		templates: templates,
		segments:  segments,
		rbac:      rbacSvc,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type AudienceInput struct {
	SourceType AudienceSourceType
	SourceID   uuid.UUID
}

type CreateInput struct {
	Name            string
	Description     *string
	ChannelID       uuid.UUID
	TemplateID      uuid.UUID
	SegmentID       *uuid.UUID
	Audiences       []AudienceInput
	VariableMapping json.RawMessage
	SampleValues    json.RawMessage
	Timezone        *string
	SendMode        SendMode
	ScheduledAt     *time.Time
}

type CreateResult struct {
	Campaign *Campaign
	Job      *BroadcastJob
	Snapshot *AudienceSnapshot
}

type UpdateInput struct {
	Name            *string
	Description     *string
	VariableMapping json.RawMessage
	SampleValues    json.RawMessage
	Timezone        *string
	SendMode        *SendMode
	ScheduledAt     *time.Time
	SegmentID       *uuid.UUID
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*CreateResult, error) {
	if err := s.authorize(actorRole, rbac.PermCampaignCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := s.validateCreateInput(ctx, teamID, input); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	campaignID := uuid.New()
	jobID := uuid.New()
	snapID := uuid.New()

	variableMapping := input.VariableMapping
	if len(variableMapping) == 0 {
		variableMapping = json.RawMessage(`{}`)
	}

	audiences := buildAudiences(campaignID, input.Audiences, now)

	initialStatus := StatusDraft
	if input.SendMode == SendLater {
		initialStatus = StatusScheduled
	}

	c := &Campaign{
		ID:              campaignID,
		TeamID:          teamID,
		ChannelID:       input.ChannelID,
		TemplateID:      input.TemplateID,
		SegmentID:       nil,
		Name:            strings.TrimSpace(input.Name),
		Description:     input.Description,
		Status:          initialStatus,
		VariableMapping: variableMapping,
		SampleValues:    input.SampleValues,
		Timezone:        input.Timezone,
		SendMode:        &input.SendMode,
		ScheduledAt:     input.ScheduledAt,
		CreatedBy:       &userID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	recipients, eligible, skippedOptOut, skippedInvalid, err := s.buildRecipientSnapshots(
		ctx,
		teamID,
		campaignID,
		jobID,
		audiences,
		variableMapping,
		input.SampleValues,
		now,
	)
	if err != nil {
		return nil, err
	}

	c.TotalContacts = eligible

	job := &BroadcastJob{
		ID:              jobID,
		CampaignID:      campaignID,
		TeamID:          teamID,
		ChannelID:       input.ChannelID,
		Status:          JobPending,
		TotalRecipients: eligible + skippedOptOut + skippedInvalid,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	sourceSummary, _ := json.Marshal(map[string]any{
		"audiences":       audiences,
		"resolved_count":  eligible,
		"skipped_opt_out": skippedOptOut,
		"skipped_invalid": skippedInvalid,
		"snapshot_at":     now,
		"frozen_at":       "create",
	})

	snap := &AudienceSnapshot{
		ID:             snapID,
		CampaignID:     campaignID,
		JobID:          jobID,
		SnapshotAt:     now,
		SourceSummary:  sourceSummary,
		ResolvedCount:  eligible,
		SkippedOptOut:  skippedOptOut,
		SkippedInvalid: skippedInvalid,
		CreatedAt:      now,
	}

	if err := s.tx.Run(ctx, func(ctx context.Context, repos Repository) error {
		if err := repos.Create(ctx, c, audiences); err != nil {
			return err
		}
		if err := repos.CreateJob(ctx, job); err != nil {
			return err
		}
		if err := repos.CreateAudienceSnapshot(ctx, snap); err != nil {
			return err
		}
		return repos.BulkCreateRecipients(ctx, recipients, now)
	}); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	return &CreateResult{Campaign: c, Job: job, Snapshot: snap}, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID) (*Campaign, error) {
	if err := s.authorize(actorRole, rbac.PermCampaignView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	c, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return c, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Campaign], error) {
	if err := s.authorize(actorRole, rbac.PermCampaignView); err != nil {
		return repository.Page[Campaign]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Campaign]{}, err
	}
	result, err := s.campaigns.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Campaign]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID, input UpdateInput) (*Campaign, error) {
	if err := s.authorize(actorRole, rbac.PermCampaignCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	c, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if c.Status != StatusDraft && c.Status != StatusScheduled {
		return nil, serviceerrors.NewValidation("status", "only draft or scheduled campaigns can be updated")
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		c.Name = name
	}
	if input.Description != nil {
		c.Description = input.Description
	}
	if len(input.VariableMapping) > 0 {
		c.VariableMapping = input.VariableMapping
	}
	if len(input.SampleValues) > 0 {
		c.SampleValues = input.SampleValues
	}
	if input.Timezone != nil {
		c.Timezone = input.Timezone
	}
	if input.SendMode != nil {
		if err := validateSendMode(*input.SendMode, coalesceTime(input.ScheduledAt, c.ScheduledAt)); err != nil {
			return nil, err
		}
		c.SendMode = input.SendMode
	}
	if input.ScheduledAt != nil {
		mode := SendLater
		if c.SendMode != nil {
			mode = *c.SendMode
		}
		if err := validateSendMode(mode, input.ScheduledAt); err != nil {
			return nil, err
		}
		c.ScheduledAt = input.ScheduledAt
	}
	if input.SegmentID != nil {
		return nil, serviceerrors.NewValidation("segment_id", "segment audiences are no longer supported; use tags or contact_ids")
	}
	c.UpdatedAt = time.Now().UTC()

	if err := serviceerrors.MapRepository(s.campaigns.Update(ctx, c)); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermCampaignCreate); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.campaigns.SoftDelete(ctx, teamID, campaignID))
}

func (s *Service) validateCreateInput(ctx context.Context, teamID uuid.UUID, input CreateInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return serviceerrors.NewValidation("name", "is required")
	}
	if err := validateChannelForCampaign(ctx, s.channels, teamID, input.ChannelID); err != nil {
		return err
	}
	if err := validateTemplateForCampaign(ctx, s.templates, teamID, input.ChannelID, input.TemplateID); err != nil {
		return err
	}
	if input.SegmentID != nil {
		return serviceerrors.NewValidation("segment_id", "segment audiences are no longer supported; use tags or contact_ids")
	}
	if len(input.Audiences) == 0 {
		return serviceerrors.NewValidation("audiences", "tag_ids or contact_ids are required")
	}
	for _, a := range input.Audiences {
		if err := validateAudienceSource(a); err != nil {
			return err
		}
	}
	return validateSendMode(input.SendMode, input.ScheduledAt)
}

func validateSendMode(mode SendMode, scheduledAt *time.Time) error {
	switch mode {
	case SendNow:
		return nil
	case SendLater:
		if scheduledAt == nil {
			return serviceerrors.NewValidation("scheduled_at", "is required when send_mode is later")
		}
		if scheduledAt.Before(time.Now().UTC()) {
			return serviceerrors.NewValidation("scheduled_at", "must be in the future")
		}
		return nil
	default:
		return serviceerrors.NewValidation("send_mode", "must be now or later")
	}
}

func validateAudienceSource(a AudienceInput) error {
	switch a.SourceType {
	case AudienceTag, AudienceContact:
		if a.SourceID == uuid.Nil {
			return serviceerrors.NewValidation("audiences.source_id", "is required")
		}
		return nil
	case AudienceSegment:
		return serviceerrors.NewValidation("audiences", "segment audiences are no longer supported; use tags or contact_ids")
	default:
		return serviceerrors.NewValidation("audiences.source_type", "invalid value")
	}
}

func buildAudiences(campaignID uuid.UUID, inputs []AudienceInput, now time.Time) []Audience {
	audiences := make([]Audience, 0, len(inputs))
	seen := map[string]struct{}{}
	for _, in := range inputs {
		if in.SourceType == AudienceSegment {
			continue
		}
		key := string(in.SourceType) + ":" + in.SourceID.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		audiences = append(audiences, Audience{
			ID:         uuid.New(),
			CampaignID: campaignID,
			SourceType: in.SourceType,
			SourceID:   in.SourceID,
			CreatedAt:  now,
		})
	}
	return audiences
}

func coalesceTime(primary, fallback *time.Time) *time.Time {
	if primary != nil {
		return primary
	}
	return fallback
}
