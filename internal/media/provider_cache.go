package media

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProviderKind identifies an external messaging/media provider without SDK coupling.
type ProviderKind string

const (
	ProviderWhatsApp  ProviderKind = "whatsapp"
	ProviderInstagram ProviderKind = "instagram"
	ProviderMessenger ProviderKind = "messenger"
)

// CacheState is the lifecycle of a ProviderMediaCache row.
type CacheState string

const (
	CacheStateActive     CacheState = "ACTIVE"
	CacheStateRefreshing CacheState = "REFRESHING"
	CacheStateFailed     CacheState = "FAILED"
)

// CacheKey uniquely identifies a cached provider media id.
type CacheKey struct {
	Provider      ProviderKind
	AssetID       uuid.UUID
	PhoneNumberID string
}

// Normalize returns a trimmed, validated-shaped key (does not check UUID nil).
func (k CacheKey) Normalize() CacheKey {
	return CacheKey{
		Provider:      ProviderKind(strings.ToLower(strings.TrimSpace(string(k.Provider)))),
		AssetID:       k.AssetID,
		PhoneNumberID: strings.TrimSpace(k.PhoneNumberID),
	}
}

// ProviderMediaCache stores a provider media id for one asset + sender identity.
// Refresh is driven by uploaded_at + MediaCachePolicy (no expires_at).
type ProviderMediaCache struct {
	ID               uuid.UUID
	TeamID           uuid.UUID
	Provider         ProviderKind
	AssetID          uuid.UUID
	PhoneNumberID    string
	ProviderMediaID  string
	State            CacheState
	LastError        *string
	UploadedAt       time.Time
	LastVerifiedAt   *time.Time
	RefreshStartedAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Key returns the unique cache key for this row.
func (c ProviderMediaCache) Key() CacheKey {
	return CacheKey{
		Provider:      c.Provider,
		AssetID:       c.AssetID,
		PhoneNumberID: c.PhoneNumberID,
	}.Normalize()
}

// HasProviderMediaID reports whether a usable provider media id is present.
func (c ProviderMediaCache) HasProviderMediaID() bool {
	return strings.TrimSpace(c.ProviderMediaID) != ""
}
