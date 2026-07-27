package media

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
)

// StorageClientObjectStore adapts storageclient.Client to ObjectStore.
type StorageClientObjectStore struct {
	client *storageclient.Client
}

func NewStorageClientObjectStore(client *storageclient.Client) *StorageClientObjectStore {
	return &StorageClientObjectStore{client: client}
}

func (s *StorageClientObjectStore) GetMetadata(ctx context.Context, teamID, objectID uuid.UUID) (*ObjectMeta, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("storage client not configured")
	}
	obj, err := s.client.GetMetadata(ctx, teamID, objectID)
	if err != nil {
		return nil, err
	}
	return mapStoredObject(obj), nil
}

func (s *StorageClientObjectStore) Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *ObjectMeta, error) {
	if s == nil || s.client == nil {
		return nil, nil, fmt.Errorf("storage client not configured")
	}
	body, obj, err := s.client.Download(ctx, teamID, objectID)
	if err != nil {
		return nil, nil, err
	}
	return body, mapStoredObject(obj), nil
}

func mapStoredObject(obj *storageclient.StoredObject) *ObjectMeta {
	if obj == nil {
		return nil
	}
	return &ObjectMeta{
		ID:          obj.ID,
		TeamID:      obj.TeamID,
		ObjectKey:   strings.TrimSpace(obj.ObjectKey),
		MimeType:    strings.TrimSpace(obj.MimeType),
		SizeBytes:   obj.SizeBytes,
		ContentHash: strings.TrimSpace(obj.Hash),
	}
}
