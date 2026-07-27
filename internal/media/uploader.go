package media

import (
	"context"
	"io"
)

// providerMediaUploader uploads binary media to an external provider.
// Only WhatsAppMediaManager (and tests) may hold an implementation — callers use EnsureMedia.
type providerMediaUploader interface {
	Upload(
		ctx context.Context,
		phoneNumberID, accessToken, mimeType, filename string,
		body io.Reader,
		size int64,
	) (providerMediaID string, err error)
}
