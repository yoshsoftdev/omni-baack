package media

import (
	"time"

	"github.com/google/uuid"
)

// MediaAsset is the Media Manager source-of-truth metadata for a stored binary.
// ID is always the storage-service object UUID (storage_object_id) — never a new UUID.
// Binary content lives in storage-service; this row is metadata only.
type MediaAsset struct {
	ID          uuid.UUID
	TeamID      uuid.UUID
	MimeType    string
	FileSize    int64
	StoragePath string
	ContentHash string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewMediaAssetFromStorage builds an asset whose ID equals the storage object ID.
func NewMediaAssetFromStorage(
	storageObjectID, teamID uuid.UUID,
	mimeType, storagePath, contentHash string,
	fileSize int64,
	now time.Time,
) MediaAsset {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return MediaAsset{
		ID:          storageObjectID,
		TeamID:      teamID,
		MimeType:    mimeType,
		FileSize:    fileSize,
		StoragePath: storagePath,
		ContentHash: contentHash,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
