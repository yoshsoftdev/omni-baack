package media

import (
	"context"
	"fmt"
	"io"

	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// metaCloudUploader is the Meta Cloud API adapter used exclusively by WhatsAppMediaManager.
type metaCloudUploader struct {
	client *metaclient.Client
}

// NewMetaCloudUploader wraps pkg/meta.Client for WhatsAppMediaManager only.
func NewMetaCloudUploader(client *metaclient.Client) providerMediaUploader {
	return &metaCloudUploader{client: client}
}

func (u *metaCloudUploader) Upload(
	ctx context.Context,
	phoneNumberID, accessToken, mimeType, filename string,
	body io.Reader,
	size int64,
) (string, error) {
	if u == nil || u.client == nil {
		return "", fmt.Errorf("meta uploader not configured")
	}
	return u.client.UploadMedia(ctx, phoneNumberID, accessToken, mimeType, filename, body, size)
}
