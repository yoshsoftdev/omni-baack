package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WhatsAppMediaManager is the WhatsApp Cloud implementation of Manager.
// It is the only component that may invoke providerMediaUploader for Meta uploads.
type WhatsAppMediaManager struct {
	assets      AssetRepository
	cache       ProviderMediaCacheRepository
	storage     ObjectStore
	uploader    providerMediaUploader
	policy      MediaCachePolicy
	retry       UploadRetryPolicy
	refreshWait RefreshWaitPolicy
	metrics     Metrics
	log         *slog.Logger
}

// WhatsAppMediaManagerConfig wires EnsureMedia dependencies.
type WhatsAppMediaManagerConfig struct {
	Assets      AssetRepository
	Cache       ProviderMediaCacheRepository
	Storage     ObjectStore
	Uploader    providerMediaUploader
	Policy      MediaCachePolicy
	Retry       UploadRetryPolicy
	RefreshWait RefreshWaitPolicy
	Metrics     Metrics
	Log         *slog.Logger
}

// NewWhatsAppMediaManager constructs a WhatsApp-backed Media Manager.
// uploader must be created via NewMetaCloudUploader (or a test double) — not shared with Worker.
func NewWhatsAppMediaManager(cfg WhatsAppMediaManagerConfig) (*WhatsAppMediaManager, error) {
	if cfg.Assets == nil {
		return nil, fmt.Errorf("assets repository is required")
	}
	if cfg.Cache == nil {
		return nil, fmt.Errorf("provider media cache repository is required")
	}
	if cfg.Storage == nil {
		return nil, fmt.Errorf("object store is required")
	}
	if cfg.Uploader == nil {
		return nil, fmt.Errorf("provider media uploader is required")
	}
	if cfg.Policy == nil {
		cfg.Policy = DefaultWhatsAppMediaCachePolicy()
	}
	cfg.Retry = cfg.Retry.Normalize()
	cfg.RefreshWait = cfg.RefreshWait.Normalize()
	if cfg.Metrics == nil {
		cfg.Metrics = NoopMetrics{}
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &WhatsAppMediaManager{
		assets:      cfg.Assets,
		cache:       cfg.Cache,
		storage:     cfg.Storage,
		uploader:    cfg.Uploader,
		policy:      cfg.Policy,
		retry:       cfg.Retry,
		refreshWait: cfg.RefreshWait,
		metrics:     cfg.Metrics,
		log:         cfg.Log,
	}, nil
}

// RegisterAsset upserts MediaAsset metadata (id = storage_object_id).
func (m *WhatsAppMediaManager) RegisterAsset(ctx context.Context, asset MediaAsset) error {
	return NewAssetRegistry(m.assets).RegisterAsset(ctx, asset)
}

// FindAsset loads MediaAsset metadata by storage_object_id.
func (m *WhatsAppMediaManager) FindAsset(ctx context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error) {
	return NewAssetRegistry(m.assets).FindAsset(ctx, teamID, assetID)
}

// EnsureMedia is the sole public API for obtaining sendable provider media.
func (m *WhatsAppMediaManager) EnsureMedia(ctx context.Context, in EnsureInput) (*MediaReference, error) {
	if err := validateEnsureInput(in); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	log := m.log.With(
		"team_id", in.TeamID.String(),
		"asset_id", in.AssetID.String(),
		"phone_number_id", strings.TrimSpace(in.PhoneNumberID),
		"force_refresh", in.ForceRefresh,
		"provider", string(ProviderWhatsApp),
	)

	asset, err := m.resolveAsset(ctx, in.TeamID, in.AssetID)
	if err != nil {
		return nil, err
	}

	key := CacheKey{
		Provider:      ProviderWhatsApp,
		AssetID:       in.AssetID,
		PhoneNumberID: strings.TrimSpace(in.PhoneNumberID),
	}.Normalize()

	if !in.ForceRefresh {
		if ref, ok := m.tryCacheHit(ctx, key, asset, now, log); ok {
			return ref, nil
		}
	}

	entry, claimed, err := m.cache.ClaimRefresh(ctx, in.TeamID, key, now, StaleRefreshLockTimeout)
	if err != nil {
		return nil, err
	}
	if !claimed {
		log.Info("media ensure waiting for in-flight refresh")
		return m.waitForRefresh(ctx, key, asset, now, log)
	}

	wasRefresh := entry.HasProviderMediaID() || in.ForceRefresh ||
		(entry.UploadedAt.Before(now) && m.policy.NeedsRefresh(*entry, now))
	source := MediaSourceCacheMissUpload
	if wasRefresh {
		source = MediaSourceRefresh
		m.metrics.IncRefresh()
	} else {
		m.metrics.IncCacheMiss()
	}

	mediaID, uploadErr := m.downloadAndUpload(ctx, in, asset, log)
	if uploadErr != nil {
		m.metrics.IncUploadFailure()
		_ = m.cache.FailRefresh(ctx, entry.ID, uploadErr.Error(), time.Now().UTC())
		log.Error("media ensure upload failed", "error", uploadErr, "source", string(source))
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, uploadErr)
	}

	completedAt := time.Now().UTC()
	if err := m.cache.CompleteRefresh(ctx, entry.ID, mediaID, completedAt); err != nil {
		return nil, err
	}
	m.metrics.IncUpload()
	log.Info("media ensure upload succeeded",
		"source", string(source),
		"provider_media_id", mediaID,
	)

	return &MediaReference{
		AssetID:         asset.ID,
		ProviderMediaID: mediaID,
		MimeType:        asset.MimeType,
		Source:          source,
	}, nil
}

func validateEnsureInput(in EnsureInput) error {
	if in.TeamID == uuid.Nil {
		return fmt.Errorf("team_id is required")
	}
	if in.AssetID == uuid.Nil {
		return ErrInvalidAssetID
	}
	if strings.TrimSpace(in.PhoneNumberID) == "" {
		return fmt.Errorf("phone_number_id is required")
	}
	if strings.TrimSpace(in.AccessToken) == "" {
		return fmt.Errorf("access_token is required")
	}
	return nil
}

func (m *WhatsAppMediaManager) resolveAsset(ctx context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error) {
	asset, err := m.assets.FindByID(ctx, teamID, assetID)
	if err == nil {
		return asset, nil
	}
	if !errors.Is(err, ErrAssetNotFound) {
		return nil, err
	}
	meta, metaErr := m.storage.GetMetadata(ctx, teamID, assetID)
	if metaErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrAssetNotFound, metaErr)
	}
	assetValue := NewMediaAssetFromStorage(
		meta.ID, teamID, meta.MimeType, meta.ObjectKey, meta.ContentHash, meta.SizeBytes, time.Now().UTC(),
	)
	if regErr := NewAssetRegistry(m.assets).RegisterAsset(ctx, assetValue); regErr != nil {
		return nil, regErr
	}
	return m.assets.FindByID(ctx, teamID, assetID)
}

func (m *WhatsAppMediaManager) tryCacheHit(
	ctx context.Context,
	key CacheKey,
	asset *MediaAsset,
	now time.Time,
	log *slog.Logger,
) (*MediaReference, bool) {
	entry, err := m.cache.FindByKey(ctx, key)
	if err != nil {
		return nil, false
	}
	if entry.State != CacheStateActive || !entry.HasProviderMediaID() {
		return nil, false
	}
	if m.policy.NeedsRefresh(*entry, now) {
		return nil, false
	}
	m.metrics.IncCacheHit()
	log.Info("media ensure cache hit",
		"source", string(MediaSourceCacheHit),
		"provider_media_id", entry.ProviderMediaID,
		"uploaded_at", entry.UploadedAt,
	)
	return &MediaReference{
		AssetID:         asset.ID,
		ProviderMediaID: entry.ProviderMediaID,
		MimeType:        asset.MimeType,
		Source:          MediaSourceCacheHit,
	}, true
}

func (m *WhatsAppMediaManager) waitForRefresh(
	ctx context.Context,
	key CacheKey,
	asset *MediaAsset,
	now time.Time,
	log *slog.Logger,
) (*MediaReference, error) {
	deadline := time.Now().Add(m.refreshWait.MaxWait)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, err := m.cache.FindByKey(ctx, key)
		if err == nil &&
			entry.State == CacheStateActive &&
			entry.HasProviderMediaID() &&
			!m.policy.NeedsRefresh(*entry, time.Now().UTC()) {
			m.metrics.IncCacheHit()
			log.Info("media ensure cache hit after wait",
				"source", string(MediaSourceCacheHit),
				"provider_media_id", entry.ProviderMediaID,
			)
			return &MediaReference{
				AssetID:         asset.ID,
				ProviderMediaID: entry.ProviderMediaID,
				MimeType:        asset.MimeType,
				Source:          MediaSourceCacheHit,
			}, nil
		}
		if time.Now().After(deadline) {
			log.Warn("media ensure refresh wait timed out")
			return nil, ErrRefreshInProgress
		}
		timer := time.NewTimer(m.refreshWait.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		_ = now
	}
}

func (m *WhatsAppMediaManager) downloadAndUpload(
	ctx context.Context,
	in EnsureInput,
	asset *MediaAsset,
	log *slog.Logger,
) (string, error) {
	body, meta, err := m.storage.Download(ctx, in.TeamID, asset.ID)
	if err != nil {
		return "", err
	}
	defer body.Close()

	mimeType := strings.TrimSpace(asset.MimeType)
	if mimeType == "" && meta != nil {
		mimeType = meta.MimeType
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	filename := filepath.Base(strings.TrimSpace(asset.StoragePath))
	if filename == "" || filename == "." || filename == "/" {
		filename = asset.ID.String()
	}
	size := asset.FileSize
	if size <= 0 && meta != nil {
		size = meta.SizeBytes
	}

	return m.uploadWithRetry(ctx, in.PhoneNumberID, in.AccessToken, mimeType, filename, body, size, log)
}

func (m *WhatsAppMediaManager) uploadWithRetry(
	ctx context.Context,
	phoneNumberID, accessToken, mimeType, filename string,
	body io.Reader,
	size int64,
	log *slog.Logger,
) (string, error) {
	// Buffer once so retries can re-read.
	data, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("read storage object: %w", err)
	}
	if size <= 0 {
		size = int64(len(data))
	}

	var lastErr error
	for attempt := 1; attempt <= m.retry.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		mediaID, uploadErr := m.uploader.Upload(
			ctx,
			phoneNumberID,
			accessToken,
			mimeType,
			filename,
			bytes.NewReader(data),
			size,
		)
		if uploadErr == nil {
			if attempt > 1 {
				log.Info("media upload succeeded after retry", "attempt", attempt)
			}
			return mediaID, nil
		}
		lastErr = uploadErr
		retryable := m.retry.IsTransient(uploadErr)
		log.Warn("media upload attempt failed",
			"attempt", attempt,
			"max_attempts", m.retry.MaxAttempts,
			"retryable", retryable,
			"error", uploadErr,
		)
		if !retryable || attempt == m.retry.MaxAttempts {
			break
		}
		delay := m.retry.BaseDelay * time.Duration(attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", lastErr
}

// Compile-time assertion: WhatsAppMediaManager implements Manager.
var _ Manager = (*WhatsAppMediaManager)(nil)
