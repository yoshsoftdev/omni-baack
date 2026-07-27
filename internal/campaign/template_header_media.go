package campaign

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/media"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

// TemplateHeaderMedia holds default header media stored on a synced template.
type TemplateHeaderMedia struct {
	Handle          string
	StorageObjectID string
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

// ExtractTemplateHeaderMediaFromTemplate prefers component storage_object_id, then metadata JSONB.
func ExtractTemplateHeaderMediaFromTemplate(tpl *template.Template) (TemplateHeaderMedia, bool) {
	if tpl == nil {
		return TemplateHeaderMedia{}, false
	}
	out, ok := ExtractTemplateHeaderMedia(tpl.Components)
	if ok && strings.TrimSpace(out.StorageObjectID) != "" {
		return out, true
	}
	if ids := tpl.Metadata.StorageObjectIDs(); len(ids) > 0 {
		out.StorageObjectID = strings.TrimSpace(ids[0])
		return out, out.StorageObjectID != "" || out.Handle != ""
	}
	return out, ok
}

func headerMappingHasMediaLink(header *HeaderMapping) bool {
	return header != nil && strings.TrimSpace(header.MediaLink) != ""
}

func headerMappingHasCloudMediaID(header *HeaderMapping) bool {
	return header != nil && IsWhatsAppCloudMediaID(header.MediaID)
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

// TemplateHasStorageBackedHeaderMedia reports whether send can resolve media via Media Manager.
func TemplateHasStorageBackedHeaderMedia(tpl *template.Template) bool {
	if tpl == nil {
		return false
	}
	defaults, ok := ExtractTemplateHeaderMediaFromTemplate(tpl)
	return ok && strings.TrimSpace(defaults.StorageObjectID) != ""
}

// ApplyTemplateHeaderMediaDefaults no longer copies Meta template header_handle into
// send-time media_id. Those handles are for template creation only and Meta rejects
// them as image.id (must be integer Cloud API media id or a public link).
// Storage-backed defaults are resolved in ResolveHeaderMediaForSend via EnsureMedia.
func ApplyTemplateHeaderMediaDefaults(mapping VariableMapping, tpl *template.Template) VariableMapping {
	_ = tpl
	return mapping
}

// ResolveHeaderMediaInput carries credentials and Media Manager for header resolution.
type ResolveHeaderMediaInput struct {
	TeamID        uuid.UUID
	PhoneNumberID string
	AccessToken   string
	Manager       media.Manager
	Metrics       media.Metrics
	Log           *slog.Logger
	ForceRefresh  bool
}

// ResolveHeaderMediaForSend is a thin adapter from campaign send mapping → Media Manager.
// Priority:
//  1. storage_object_id → EnsureMedia (primary)
//  2. existing Cloud media_id on mapping
//  3. media_link (deprecated compatibility path)
//
// Worker must not upload to Meta; all WhatsApp Cloud uploads go through EnsureMedia.
func ResolveHeaderMediaForSend(
	ctx context.Context,
	mapping VariableMapping,
	tpl *template.Template,
	in ResolveHeaderMediaInput,
) (VariableMapping, error) {
	if tpl == nil {
		return mapping, nil
	}
	log := in.Log
	if log == nil {
		log = slog.Default()
	}
	metrics := in.Metrics
	if metrics == nil {
		metrics = media.NoopMetrics{}
	}

	// Drop non-sendable values (template header_handle mistakenly stored as media_id).
	if mapping.Header != nil && strings.TrimSpace(mapping.Header.MediaID) != "" && !IsWhatsAppCloudMediaID(mapping.Header.MediaID) {
		cleared := *mapping.Header
		cleared.MediaID = ""
		mapping.Header = &cleared
	}

	defaults, ok := ExtractTemplateHeaderMediaFromTemplate(tpl)
	storageObjectID := ""
	if ok {
		storageObjectID = strings.TrimSpace(defaults.StorageObjectID)
	}

	// Primary path: storage-backed asset via Media Manager.EnsureMedia.
	if storageObjectID != "" && in.Manager != nil {
		assetID, err := uuid.Parse(storageObjectID)
		if err != nil {
			log.Warn("template storage_object_id is not a UUID; skipping EnsureMedia",
				"storage_object_id", storageObjectID,
			)
		} else {
			ref, err := in.Manager.EnsureMedia(ctx, media.EnsureInput{
				TeamID:        in.TeamID,
				AssetID:       assetID,
				PhoneNumberID: strings.TrimSpace(in.PhoneNumberID),
				AccessToken:   strings.TrimSpace(in.AccessToken),
				ForceRefresh:  in.ForceRefresh,
			})
			if err != nil {
				return mapping, err
			}
			if mapping.Header == nil {
				mapping.Header = &HeaderMapping{}
			}
			mapping.Header.MediaID = ref.ProviderMediaID
			mapping.Header.MediaLink = ""
			log.Info("header media resolved via EnsureMedia",
				"asset_id", assetID.String(),
				"source", string(ref.Source),
				"provider_media_id", ref.ProviderMediaID,
			)
			return mapping, nil
		}
	}

	// Already have a Cloud media id on the campaign mapping.
	if headerMappingHasCloudMediaID(mapping.Header) {
		return mapping, nil
	}

	// Secondary compatibility path: public media_link (deprecated).
	if headerMappingHasMediaLink(mapping.Header) {
		metrics.IncDeprecatedLink()
		log.Warn("using deprecated media_link for template header; prefer storage_object_id → EnsureMedia",
			"media_link", strings.TrimSpace(mapping.Header.MediaLink),
		)
		return mapping, nil
	}

	return mapping, nil
}
