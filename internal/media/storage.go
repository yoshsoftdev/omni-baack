package media

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// ObjectMeta is storage metadata needed to upload to a provider.
type ObjectMeta struct {
	ID          uuid.UUID
	TeamID      uuid.UUID
	ObjectKey   string
	MimeType    string
	SizeBytes   int64
	ContentHash string
}

// ObjectStore reads binaries from internal storage (source of truth).
type ObjectStore interface {
	GetMetadata(ctx context.Context, teamID, objectID uuid.UUID) (*ObjectMeta, error)
	Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *ObjectMeta, error)
}
