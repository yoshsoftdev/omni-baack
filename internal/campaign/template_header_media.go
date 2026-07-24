package campaign

import (
	"context"
	"encoding/json"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

// TemplateHeaderMedia holds default header media stored on a synced template.
type TemplateHeaderMedia struct {
	Handle          string
	StorageObjectID string
}

// HeaderMediaUploader uploads binary media to Meta for template sends.
type HeaderMediaUploader interface {
	UploadMedia(ctx context.Context, phoneNumberID, accessToken, mimeType, filename string, body io.Reader, size int64) (string, error)
}

// HeaderStorageDownloader fetches media previously uploaded to storage-service.
type HeaderStorageDownloader interface {
	Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *storageclient.StoredObject, error)
}

// ExtractTemplateHeaderMedia reads default media metadata from template components.
func ExtractTemplateHeaderMedia(components json.RawMessage) (TemplateHeaderMedia, bool) {
	if len(components) == 0 {
		return TemplateHeaderMedia{}, false
	}

	var items []map[string]any
	if err := json.Unmarshal(components, &items); err != nil {
		return TemplateHeaderMedia{}, false
	}

	for _, item := range items {
		typeRaw, _ := item["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typeRaw), "HEADER") {
			continue
		}
		format, _ := item["format"].(string)
		switch strings.ToUpper(strings.TrimSpace(format)) {
		case "IMAGE", "VIDEO", "DOCUMENT":
		default:
			continue
		}

		example, _ := item["example"].(map[string]any)
		if example == nil {
			return TemplateHeaderMedia{}, false
		}

		out := TemplateHeaderMedia{}
		if storageIDs, ok := example["storage_object_id"].([]any); ok && len(storageIDs) > 0 {
			if value, ok := storageIDs[0].(string); ok {
				out.StorageObjectID = strings.TrimSpace(value)
			}
		}
		if handles, ok := example["header_handle"].([]any); ok && len(handles) > 0 {
			if value, ok := handles[0].(string); ok {
				out.Handle = strings.TrimSpace(value)
			}
		}
		if out.Handle == "" && out.StorageObjectID == "" {
			return TemplateHeaderMedia{}, false
		}
		return out, true
	}

	return TemplateHeaderMedia{}, false
}

func headerMappingHasSendableMedia(header *HeaderMapping) bool {
	if header == nil {
		return false
	}
	if strings.TrimSpace(header.MediaLink) != "" {
		return true
	}
	return IsWhatsAppCloudMediaID(header.MediaID)
}

// IsWhatsAppCloudMediaID reports whether id is a Cloud API media id (digits only).
// Template creation header_handle values (e.g. "4:...") must never be sent as image.id.
func IsWhatsAppCloudMediaID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, r := range id {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// TemplateHasStorageBackedHeaderMedia reports whether send can upload media from storage.
func TemplateHasStorageBackedHeaderMedia(tpl *template.Template) bool {
	if tpl == nil {
		return false
	}
	defaults, ok := ExtractTemplateHeaderMedia(tpl.Components)
	return ok && strings.TrimSpace(defaults.StorageObjectID) != ""
}

// ApplyTemplateHeaderMediaDefaults no longer copies Meta template header_handle into
// send-time media_id. Those handles are for template creation only and Meta rejects
// them as image.id (must be integer Cloud API media id or a public link).
// Storage-backed defaults are resolved in ResolveHeaderMediaForSend.
func ApplyTemplateHeaderMediaDefaults(mapping VariableMapping, tpl *template.Template) VariableMapping {
	_ = tpl
	return mapping
}

// ResolveHeaderMediaForSend uploads storage-backed header media when needed.
func ResolveHeaderMediaForSend(
	ctx context.Context,
	mapping VariableMapping,
	tpl *template.Template,
	teamID uuid.UUID,
	phoneNumberID, accessToken string,
	storage HeaderStorageDownloader,
	uploader HeaderMediaUploader,
) (VariableMapping, error) {
	if tpl == nil {
		return mapping, nil
	}

	if headerMappingHasSendableMedia(mapping.Header) {
		return mapping, nil
	}

	// Drop non-sendable values (template header_handle mistakenly stored as media_id).
	if mapping.Header != nil && strings.TrimSpace(mapping.Header.MediaID) != "" && !IsWhatsAppCloudMediaID(mapping.Header.MediaID) {
		cleared := *mapping.Header
		cleared.MediaID = ""
		mapping.Header = &cleared
	}

	defaults, ok := ExtractTemplateHeaderMedia(tpl.Components)
	if !ok || strings.TrimSpace(defaults.StorageObjectID) == "" {
		return mapping, nil
	}
	if storage == nil || uploader == nil {
		return mapping, nil
	}

	objectID, err := uuid.Parse(strings.TrimSpace(defaults.StorageObjectID))
	if err != nil {
		return mapping, nil
	}

	reader, meta, err := storage.Download(ctx, teamID, objectID)
	if err != nil {
		return mapping, err
	}
	defer reader.Close()

	mimeType := strings.TrimSpace(meta.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	fileName := path.Base(strings.TrimSpace(meta.ObjectKey))
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "template-header"
	}

	mediaID, err := uploader.UploadMedia(ctx, phoneNumberID, accessToken, mimeType, fileName, reader, meta.SizeBytes)
	if err != nil {
		return mapping, err
	}

	if mapping.Header == nil {
		mapping.Header = &HeaderMapping{}
	}
	mapping.Header.MediaID = mediaID
	mapping.Header.MediaLink = ""
	return mapping, nil
}
