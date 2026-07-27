package media

import "github.com/google/uuid"

// MediaSource describes how a MediaReference was obtained (observability).
type MediaSource string

const (
	MediaSourceCacheHit       MediaSource = "cache_hit"
	MediaSourceCacheMissUpload MediaSource = "cache_miss_upload"
	MediaSourceRefresh        MediaSource = "refresh"
	MediaSourceDeprecatedLink MediaSource = "deprecated_link"
)

// MediaReference is the provider-agnostic handle returned by Media Manager.
// Callers (Template Builder / Worker) consume this — not raw upload details.
type MediaReference struct {
	AssetID         uuid.UUID
	ProviderMediaID string
	MimeType        string
	Source          MediaSource
	// DeprecatedLink is set only when Source == MediaSourceDeprecatedLink.
	DeprecatedLink string
}

// UsesProviderMediaID reports whether the reference carries a provider media id.
func (r MediaReference) UsesProviderMediaID() bool {
	return r.ProviderMediaID != ""
}

// UsesDeprecatedLink reports whether the secondary public-link path was used.
func (r MediaReference) UsesDeprecatedLink() bool {
	return r.Source == MediaSourceDeprecatedLink && r.DeprecatedLink != ""
}
