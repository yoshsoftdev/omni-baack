package media

import "errors"

var (
	// ErrAssetNotFound is returned when MediaAsset metadata is missing.
	ErrAssetNotFound = errors.New("media asset not found")

	// ErrInvalidAssetID is returned when asset id is nil or does not match storage_object_id rules.
	ErrInvalidAssetID = errors.New("media asset id must equal storage_object_id")

	// ErrUploadFailed is returned when provider upload fails (cache is not cleared).
	ErrUploadFailed = errors.New("failed to upload media to Meta")

	// ErrCacheNotFound is returned when no provider media cache row exists for the key.
	ErrCacheNotFound = errors.New("provider media cache not found")

	// ErrRefreshInProgress is returned when another caller holds a non-stale REFRESHING lock.
	ErrRefreshInProgress = errors.New("provider media refresh already in progress")
)
