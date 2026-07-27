package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// ProviderMediaCacheRepository persists provider media id cache rows.
type ProviderMediaCacheRepository interface {
	// FindByKey loads a cache row by (provider, asset_id, phone_number_id).
	FindByKey(ctx context.Context, key CacheKey) (*ProviderMediaCache, error)

	// Upsert inserts or updates by unique (provider, asset_id, phone_number_id).
	Upsert(ctx context.Context, entry ProviderMediaCache) error

	// ClaimRefresh transitions a row to REFRESHING for single-flight uploads.
	// claimed=true means this caller owns the refresh.
	// claimed=false means another caller holds a non-stale REFRESHING lock.
	// If no row exists, creates a REFRESHING placeholder (provider_media_id empty).
	ClaimRefresh(ctx context.Context, teamID uuid.UUID, key CacheKey, now time.Time, staleAfter time.Duration) (entry *ProviderMediaCache, claimed bool, err error)

	// CompleteRefresh marks ACTIVE with a new provider media id after successful upload.
	CompleteRefresh(ctx context.Context, id uuid.UUID, providerMediaID string, now time.Time) error

	// FailRefresh marks FAILED with last_error; existing provider_media_id is preserved.
	FailRefresh(ctx context.Context, id uuid.UUID, lastError string, now time.Time) error
}

// PostgresProviderMediaCacheRepository stores provider_media_cache in PostgreSQL.
type PostgresProviderMediaCacheRepository struct {
	db *database.DB
}

func NewPostgresProviderMediaCacheRepository(db *database.DB) *PostgresProviderMediaCacheRepository {
	return &PostgresProviderMediaCacheRepository{db: db}
}

func (r *PostgresProviderMediaCacheRepository) FindByKey(ctx context.Context, key CacheKey) (*ProviderMediaCache, error) {
	key = key.Normalize()
	if err := validateCacheKey(key); err != nil {
		return nil, err
	}
	row := r.db.Pool.QueryRow(ctx, providerMediaCacheSelect+`
		WHERE provider = $1 AND asset_id = $2 AND phone_number_id = $3
	`, string(key.Provider), key.AssetID, key.PhoneNumberID)
	entry, err := scanProviderMediaCache(row)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrCacheNotFound
		}
		return nil, err
	}
	return entry, nil
}

func (r *PostgresProviderMediaCacheRepository) Upsert(ctx context.Context, entry ProviderMediaCache) error {
	if err := validateCacheEntry(entry); err != nil {
		return err
	}
	key := entry.Key()
	now := time.Now().UTC()
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.UploadedAt.IsZero() {
		entry.UploadedAt = now
	}
	if entry.State == "" {
		entry.State = CacheStateActive
	}

	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO provider_media_cache (
			id, team_id, provider, asset_id, phone_number_id, provider_media_id,
			state, last_error, uploaded_at, last_verified_at, refresh_started_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (provider, asset_id, phone_number_id) DO UPDATE SET
			team_id = EXCLUDED.team_id,
			provider_media_id = EXCLUDED.provider_media_id,
			state = EXCLUDED.state,
			last_error = EXCLUDED.last_error,
			uploaded_at = EXCLUDED.uploaded_at,
			last_verified_at = EXCLUDED.last_verified_at,
			refresh_started_at = EXCLUDED.refresh_started_at,
			updated_at = EXCLUDED.updated_at
	`, entry.ID, entry.TeamID, string(key.Provider), key.AssetID, key.PhoneNumberID, entry.ProviderMediaID,
		string(entry.State), entry.LastError, entry.UploadedAt, entry.LastVerifiedAt, entry.RefreshStartedAt,
		entry.CreatedAt, entry.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresProviderMediaCacheRepository) ClaimRefresh(
	ctx context.Context,
	teamID uuid.UUID,
	key CacheKey,
	now time.Time,
	staleAfter time.Duration,
) (*ProviderMediaCache, bool, error) {
	key = key.Normalize()
	if err := validateCacheKey(key); err != nil {
		return nil, false, err
	}
	if teamID == uuid.Nil {
		return nil, false, fmt.Errorf("team_id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if staleAfter <= 0 {
		staleAfter = StaleRefreshLockTimeout
	}
	staleBefore := now.Add(-staleAfter)

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, providerMediaCacheSelect+`
		WHERE provider = $1 AND asset_id = $2 AND phone_number_id = $3
		FOR UPDATE
	`, string(key.Provider), key.AssetID, key.PhoneNumberID)
	existing, err := scanProviderMediaCache(row)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	if existing == nil {
		id := uuid.New()
		_, err = tx.Exec(ctx, `
			INSERT INTO provider_media_cache (
				id, team_id, provider, asset_id, phone_number_id, provider_media_id,
				state, last_error, uploaded_at, refresh_started_at, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,'','REFRESHING',NULL,$6,$6,$6,$6)
		`, id, teamID, string(key.Provider), key.AssetID, key.PhoneNumberID, now)
		if err != nil {
			return nil, false, repository.MapError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return &ProviderMediaCache{
			ID:               id,
			TeamID:           teamID,
			Provider:         key.Provider,
			AssetID:          key.AssetID,
			PhoneNumberID:    key.PhoneNumberID,
			State:            CacheStateRefreshing,
			UploadedAt:       now,
			RefreshStartedAt: &now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}, true, nil
	}

	if existing.State == CacheStateRefreshing &&
		existing.RefreshStartedAt != nil &&
		existing.RefreshStartedAt.After(staleBefore) {
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE provider_media_cache SET
			state = 'REFRESHING',
			refresh_started_at = $1,
			updated_at = $1,
			last_error = NULL
		WHERE id = $2
	`, now, existing.ID)
	if err != nil {
		return nil, false, repository.MapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	existing.State = CacheStateRefreshing
	existing.RefreshStartedAt = &now
	existing.UpdatedAt = now
	existing.LastError = nil
	return existing, true, nil
}

func (r *PostgresProviderMediaCacheRepository) CompleteRefresh(ctx context.Context, id uuid.UUID, providerMediaID string, now time.Time) error {
	providerMediaID = strings.TrimSpace(providerMediaID)
	if id == uuid.Nil {
		return fmt.Errorf("cache id is required")
	}
	if providerMediaID == "" {
		return fmt.Errorf("provider_media_id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE provider_media_cache SET
			provider_media_id = $1,
			state = 'ACTIVE',
			last_error = NULL,
			uploaded_at = $2,
			last_verified_at = $2,
			refresh_started_at = NULL,
			updated_at = $2
		WHERE id = $3
	`, providerMediaID, now, id)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCacheNotFound
	}
	return nil
}

func (r *PostgresProviderMediaCacheRepository) FailRefresh(ctx context.Context, id uuid.UUID, lastError string, now time.Time) error {
	if id == uuid.Nil {
		return fmt.Errorf("cache id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	msg := strings.TrimSpace(lastError)
	var errPtr *string
	if msg != "" {
		errPtr = &msg
	}
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE provider_media_cache SET
			state = 'FAILED',
			last_error = $1,
			refresh_started_at = NULL,
			updated_at = $2
		WHERE id = $3
	`, errPtr, now, id)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCacheNotFound
	}
	return nil
}

const providerMediaCacheSelect = `
	SELECT id, team_id, provider, asset_id, phone_number_id, provider_media_id,
		state, last_error, uploaded_at, last_verified_at, refresh_started_at, created_at, updated_at
	FROM provider_media_cache
`

func scanProviderMediaCache(row pgx.Row) (*ProviderMediaCache, error) {
	var e ProviderMediaCache
	var provider, state string
	err := row.Scan(
		&e.ID, &e.TeamID, &provider, &e.AssetID, &e.PhoneNumberID, &e.ProviderMediaID,
		&state, &e.LastError, &e.UploadedAt, &e.LastVerifiedAt, &e.RefreshStartedAt, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan provider media cache: %w", err)
	}
	e.Provider = ProviderKind(provider)
	e.State = CacheState(state)
	return &e, nil
}

func validateCacheKey(key CacheKey) error {
	key = key.Normalize()
	if key.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if key.AssetID == uuid.Nil {
		return ErrInvalidAssetID
	}
	if key.PhoneNumberID == "" {
		return fmt.Errorf("phone_number_id is required")
	}
	return nil
}

func validateCacheEntry(entry ProviderMediaCache) error {
	if entry.TeamID == uuid.Nil {
		return fmt.Errorf("team_id is required")
	}
	return validateCacheKey(entry.Key())
}

// MemoryProviderMediaCacheRepository is an in-memory cache repo for unit tests.
type MemoryProviderMediaCacheRepository struct {
	mu   sync.Mutex
	byKey map[string]ProviderMediaCache
}

func NewMemoryProviderMediaCacheRepository() *MemoryProviderMediaCacheRepository {
	return &MemoryProviderMediaCacheRepository{byKey: map[string]ProviderMediaCache{}}
}

func cacheMemoryKey(key CacheKey) string {
	key = key.Normalize()
	return string(key.Provider) + "|" + key.AssetID.String() + "|" + key.PhoneNumberID
}

func (r *MemoryProviderMediaCacheRepository) FindByKey(_ context.Context, key CacheKey) (*ProviderMediaCache, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateCacheKey(key); err != nil {
		return nil, err
	}
	entry, ok := r.byKey[cacheMemoryKey(key)]
	if !ok {
		return nil, ErrCacheNotFound
	}
	cp := entry
	return &cp, nil
}

func (r *MemoryProviderMediaCacheRepository) Upsert(_ context.Context, entry ProviderMediaCache) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateCacheEntry(entry); err != nil {
		return err
	}
	key := entry.Key()
	now := time.Now().UTC()
	if entry.ID == uuid.Nil {
		if existing, ok := r.byKey[cacheMemoryKey(key)]; ok {
			entry.ID = existing.ID
			entry.CreatedAt = existing.CreatedAt
		} else {
			entry.ID = uuid.New()
			entry.CreatedAt = now
		}
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.UploadedAt.IsZero() {
		entry.UploadedAt = now
	}
	if entry.State == "" {
		entry.State = CacheStateActive
	}
	r.byKey[cacheMemoryKey(key)] = entry
	return nil
}

func (r *MemoryProviderMediaCacheRepository) ClaimRefresh(
	_ context.Context,
	teamID uuid.UUID,
	key CacheKey,
	now time.Time,
	staleAfter time.Duration,
) (*ProviderMediaCache, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key = key.Normalize()
	if err := validateCacheKey(key); err != nil {
		return nil, false, err
	}
	if teamID == uuid.Nil {
		return nil, false, fmt.Errorf("team_id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if staleAfter <= 0 {
		staleAfter = StaleRefreshLockTimeout
	}
	staleBefore := now.Add(-staleAfter)
	mk := cacheMemoryKey(key)

	existing, ok := r.byKey[mk]
	if !ok {
		entry := ProviderMediaCache{
			ID:               uuid.New(),
			TeamID:           teamID,
			Provider:         key.Provider,
			AssetID:          key.AssetID,
			PhoneNumberID:    key.PhoneNumberID,
			State:            CacheStateRefreshing,
			UploadedAt:       now,
			RefreshStartedAt: &now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		r.byKey[mk] = entry
		cp := entry
		return &cp, true, nil
	}

	if existing.State == CacheStateRefreshing &&
		existing.RefreshStartedAt != nil &&
		existing.RefreshStartedAt.After(staleBefore) {
		cp := existing
		return &cp, false, nil
	}

	existing.State = CacheStateRefreshing
	existing.RefreshStartedAt = &now
	existing.UpdatedAt = now
	existing.LastError = nil
	r.byKey[mk] = existing
	cp := existing
	return &cp, true, nil
}

func (r *MemoryProviderMediaCacheRepository) CompleteRefresh(_ context.Context, id uuid.UUID, providerMediaID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	providerMediaID = strings.TrimSpace(providerMediaID)
	if providerMediaID == "" {
		return fmt.Errorf("provider_media_id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for k, entry := range r.byKey {
		if entry.ID != id {
			continue
		}
		entry.ProviderMediaID = providerMediaID
		entry.State = CacheStateActive
		entry.LastError = nil
		entry.UploadedAt = now
		entry.LastVerifiedAt = &now
		entry.RefreshStartedAt = nil
		entry.UpdatedAt = now
		r.byKey[k] = entry
		return nil
	}
	return ErrCacheNotFound
}

func (r *MemoryProviderMediaCacheRepository) FailRefresh(_ context.Context, id uuid.UUID, lastError string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	msg := strings.TrimSpace(lastError)
	for k, entry := range r.byKey {
		if entry.ID != id {
			continue
		}
		entry.State = CacheStateFailed
		if msg != "" {
			entry.LastError = &msg
		} else {
			entry.LastError = nil
		}
		entry.RefreshStartedAt = nil
		entry.UpdatedAt = now
		r.byKey[k] = entry
		return nil
	}
	return ErrCacheNotFound
}
