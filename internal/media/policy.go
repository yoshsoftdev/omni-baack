package media

import "time"

// MediaCachePolicy decides whether a cached provider media id must be refreshed.
// Implementations must not rely on an expires_at column — use uploaded_at (and optional verification).
type MediaCachePolicy interface {
	NeedsRefresh(entry ProviderMediaCache, now time.Time) bool
}

// AgeBasedMediaCachePolicy refreshes when uploaded_at is older than MaxAge.
type AgeBasedMediaCachePolicy struct {
	MaxAge time.Duration
}

// DefaultWhatsAppMediaCachePolicy returns the WhatsApp Cloud media age policy (~28 days).
func DefaultWhatsAppMediaCachePolicy() MediaCachePolicy {
	return AgeBasedMediaCachePolicy{MaxAge: 28 * 24 * time.Hour}
}

// NeedsRefresh reports whether the cache entry should be re-uploaded.
func (p AgeBasedMediaCachePolicy) NeedsRefresh(entry ProviderMediaCache, now time.Time) bool {
	if p.MaxAge <= 0 {
		return true
	}
	if entry.UploadedAt.IsZero() {
		return true
	}
	if !entry.HasProviderMediaID() {
		return true
	}
	if entry.State == CacheStateFailed {
		return true
	}
	age := now.UTC().Sub(entry.UploadedAt.UTC())
	return age >= p.MaxAge
}

// StaleRefreshLockTimeout is the default window after which a REFRESHING lock may be reclaimed.
const StaleRefreshLockTimeout = 2 * time.Minute
