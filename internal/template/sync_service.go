package template

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const templateSyncFreshness = 24 * time.Hour

// MetaTemplatePageLister fetches paginated templates from Meta Graph API.
type MetaTemplatePageLister interface {
	ListMessageTemplatesPage(ctx context.Context, wabaID, accessToken, after string) ([]metapkg.MessageTemplate, string, error)
}

type headerMediaDownloader interface {
	Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *storageclient.StoredObject, error)
}

type resumableMediaUploader interface {
	UploadResumableMedia(
		ctx context.Context,
		appID, accessToken, fileName, mimeType string,
		body io.Reader,
		sizeBytes int64,
	) (string, error)
}

// SyncService pulls templates from Meta with pagination, retry, drift detection, and audit trail.
type SyncService struct {
	meta      MetaTemplatePageLister
	channels  channel.Repository
	templates Repository
	syncRuns  SyncRunRepository
	cipher    *cryptoutil.TokenCipher
	log       *slog.Logger
	appID     string
	storage   headerMediaDownloader
}

func NewSyncService(
	meta MetaTemplatePageLister,
	channels channel.Repository,
	templates Repository,
	syncRuns SyncRunRepository,
	cipher *cryptoutil.TokenCipher,
	log *slog.Logger,
) *SyncService {
	if log == nil {
		log = slog.Default()
	}
	return &SyncService{
		meta:      meta,
		channels:  channels,
		templates: templates,
		syncRuns:  syncRuns,
		cipher:    cipher,
		log:       log,
	}
}

// WithAppID configures Meta App ID used for resumable media uploads (template headers).
func (s *SyncService) WithAppID(appID string) *SyncService {
	s.appID = strings.TrimSpace(appID)
	return s
}

// WithStorage configures storage-service download for converting object ids to Meta header handles.
func (s *SyncService) WithStorage(storage headerMediaDownloader) *SyncService {
	s.storage = storage
	return s
}

type SyncResult struct {
	SyncRunID  uuid.UUID `json:"sync_run_id"`
	Created    int       `json:"created"`
	Updated    int       `json:"updated"`
	Removed    int64     `json:"removed"`
	Synced     int       `json:"synced"`
	ErrorCount int       `json:"error_count,omitempty"`
}

func (s *SyncService) SyncChannel(ctx context.Context, teamID, channelID uuid.UUID) (*SyncResult, error) {
	started := time.Now().UTC()
	run := &SyncRun{
		ID:        uuid.New(),
		TeamID:    teamID,
		ChannelID: channelID,
		StartedAt: started,
		Status:    SyncRunRunning,
	}
	if s.syncRuns != nil {
		_ = s.syncRuns.Create(ctx, run)
	}

	result, syncErr := s.executeSync(ctx, teamID, channelID, run)
	completed := time.Now().UTC()
	run.CompletedAt = &completed

	if syncErr != nil {
		msg := syncErr.Error()
		run.Status = SyncRunFailed
		run.ErrorMessage = &msg
		if result != nil {
			run.SyncedCount = result.Created + result.Updated
			run.CreatedCount = result.Created
			run.UpdatedCount = result.Updated
			run.RemovedCount = result.Removed
			run.ErrorCount = result.ErrorCount
		}
		if s.syncRuns != nil {
			_ = s.syncRuns.Update(ctx, run)
		}
		s.log.Error("template_sync_failed",
			"event", "template_sync_failed",
			"channel_id", channelID.String(),
			"sync_run_id", run.ID.String(),
			"duration_ms", completed.Sub(started).Milliseconds(),
			"error", msg,
		)
		return nil, syncErr
	}

	run.SyncedCount = result.Created + result.Updated
	run.CreatedCount = result.Created
	run.UpdatedCount = result.Updated
	run.RemovedCount = result.Removed
	run.ErrorCount = result.ErrorCount

	run.Status = SyncRunSuccess
	if s.syncRuns != nil {
		_ = s.syncRuns.Update(ctx, run)
	}

	wabaID := ""
	if ch, _, _, err := s.loadChannelCredentials(ctx, teamID, channelID); err == nil && ch.WABAID != nil {
		wabaID = *ch.WABAID
	}

	s.log.Info("template_sync_completed",
		"event", "template_sync_completed",
		"channel_id", channelID.String(),
		"waba_id", wabaID,
		"sync_run_id", run.ID.String(),
		"duration_ms", completed.Sub(started).Milliseconds(),
		"created_count", result.Created,
		"updated_count", result.Updated,
		"removed_count", result.Removed,
	)

	result.SyncRunID = run.ID
	return result, nil
}

func (s *SyncService) executeSync(ctx context.Context, teamID, channelID uuid.UUID, run *SyncRun) (*SyncResult, error) {
	ch, creds, token, err := s.loadChannelCredentials(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	if err := channel.ValidateForMeta(ch, creds, token); err != nil {
		return nil, err
	}

	wabaID := stringValue(ch.WABAID)
	metaTemplates, lastCursor, err := s.fetchAllTemplates(ctx, wabaID, token)
	if err != nil {
		if metapkg.IsInvalidToken(err) {
			return nil, serviceerrors.NewValidation("channel_id", "Meta access token is invalid or expired")
		}
		if metapkg.IsMissingResource(err) {
			return nil, serviceerrors.NewValidation("waba_id", "WhatsApp Business Account not found in Meta")
		}
		if isInvalidWABAForTemplates(err) {
			return nil, serviceerrors.NewValidation("waba_id", "invalid WhatsApp Business Account ID; use the WABA ID from Meta Business Manager, not Phone Number ID")
		}
		return nil, serviceerrors.NewValidation("meta", formatMetaTemplateListError(err))
	}
	if lastCursor != "" {
		run.MetaCursor = &lastCursor
	}

	result := &SyncResult{}
	keepIDs := make([]string, 0, len(metaTemplates))
	for _, mt := range metaTemplates {
		keepIDs = append(keepIDs, mt.ID)
		now := time.Now().UTC()
		// Meta DTO → domain only via mapper boundary (Phase 3).
		incoming := MapFromMeta(channelID, teamID, wabaID, mt, now)

		existing, findErr := s.templates.FindByMetaTemplateID(ctx, teamID, channelID, mt.ID)
		if findErr == nil {
			incoming = MergeSyncedTemplate(existing, incoming)
			if templateDriftChanged(existing, incoming) {
				result.Updated++
			}
		} else {
			incoming = MergeSyncedTemplate(nil, incoming)
			result.Created++
		}

		if err := s.templates.UpsertFromSync(ctx, incoming); err != nil {
			result.ErrorCount++
			s.log.Warn("template_sync_upsert_failed",
				"event", "template_sync_upsert_failed",
				"channel_id", channelID.String(),
				"sync_run_id", run.ID.String(),
				"meta_template_id", mt.ID,
			)
			return nil, serviceerrors.MapRepository(err)
		}
	}

	removed, err := s.templates.SoftDeleteExceptMetaIDs(ctx, teamID, channelID, keepIDs)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	result.Removed = removed
	result.Synced = len(metaTemplates)
	return result, nil
}

func (s *SyncService) fetchAllTemplates(ctx context.Context, wabaID, token string) ([]metapkg.MessageTemplate, string, error) {
	var all []metapkg.MessageTemplate
	after := ""
	var lastCursor string

	for {
		var page []metapkg.MessageTemplate
		var nextAfter string
		err := metapkg.Retry(ctx, func(retryCtx context.Context) error {
			var retryErr error
			page, nextAfter, retryErr = s.meta.ListMessageTemplatesPage(retryCtx, wabaID, token, after)
			return retryErr
		})
		if err != nil {
			return nil, lastCursor, err
		}
		all = append(all, page...)
		if nextAfter == "" {
			break
		}
		after = nextAfter
		lastCursor = nextAfter
	}
	return all, lastCursor, nil
}

func (s *SyncService) SyncRuns() SyncRunRepository {
	return s.syncRuns
}

func (s *SyncService) ChannelSyncFresh(ctx context.Context, channelID uuid.UUID) bool {
	if s.syncRuns == nil {
		return false
	}
	run, err := s.syncRuns.LastSuccessfulByChannel(ctx, channelID)
	if err != nil {
		return false
	}
	completedAt := run.StartedAt
	if run.CompletedAt != nil {
		completedAt = *run.CompletedAt
	}
	return time.Since(completedAt) <= templateSyncFreshness
}

func (s *SyncService) TeamRecentlySynced(ctx context.Context, teamID uuid.UUID) (bool, error) {
	if s.syncRuns == nil {
		return s.templates.HasRecentSync(ctx, teamID, templateSyncFreshness)
	}
	since := time.Now().UTC().Add(-templateSyncFreshness)
	count, err := s.syncRuns.CountSuccessfulWithin(ctx, teamID, since)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *SyncService) loadChannelCredentials(ctx context.Context, teamID, channelID uuid.UUID) (*channel.Channel, *channel.Credentials, string, error) {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, nil, "", serviceerrors.MapRepository(err)
	}
	creds, err := s.channels.FindCredentials(ctx, channelID)
	if err != nil {
		return nil, nil, "", serviceerrors.MapRepository(err)
	}
	if creds == nil || len(creds.AccessTokenEnc) == 0 {
		return nil, nil, "", serviceerrors.NewValidation("channel_id", "access token is required")
	}
	if s.cipher == nil {
		return nil, nil, "", serviceerrors.NewValidation("channel_id", "token encryption is not configured")
	}
	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return nil, nil, "", serviceerrors.NewValidation("channel_id", "stored access token is invalid")
	}
	token = strings.TrimSpace(token)
	return ch, creds, token, nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func formatMetaTemplateListError(err error) string {
	var apiErr *metapkg.APIError
	if errors.As(err, &apiErr) {
		body := strings.TrimSpace(apiErr.Body)
		if body != "" {
			return fmt.Sprintf("failed to list templates from Meta (HTTP %d): %s", apiErr.StatusCode, body)
		}
		return fmt.Sprintf("failed to list templates from Meta (HTTP %d)", apiErr.StatusCode)
	}
	return fmt.Sprintf("failed to list templates from Meta: %v", err)
}

func isInvalidWABAForTemplates(err error) bool {
	var apiErr *metapkg.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		return false
	}
	body := strings.ToLower(apiErr.Body)
	return strings.Contains(body, "message_templates") ||
		strings.Contains(body, "nonexisting field")
}
