package template

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

var uuidLikeHandle = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// CreateInput is the application input for creating a Meta message template.
type CreateInput struct {
	ChannelID  uuid.UUID
	Name       string
	Language   string
	Category   string
	Components json.RawMessage
}

// Create submits a new template to Meta, then upserts the PENDING row locally.
func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Template, error) {
	if err := s.authorize(actorRole, rbac.PermTemplateCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.sync == nil {
		return nil, serviceerrors.NewValidation("create", "template create is not configured")
	}

	name := strings.TrimSpace(strings.ToLower(input.Name))
	if name == "" || !isValidTemplateName(name) {
		return nil, serviceerrors.NewValidation("name", "template name must use lowercase letters, numbers, and underscores")
	}
	language := normalizeCreateLanguage(input.Language)
	category, err := normalizeCreateCategory(input.Category)
	if err != nil {
		return nil, err
	}
	components, err := normalizeCreateComponents(input.Components)
	if err != nil {
		return nil, err
	}

	return s.sync.CreateOnMeta(ctx, teamID, input.ChannelID, name, language, category, components)
}

func (s *SyncService) CreateOnMeta(
	ctx context.Context,
	teamID, channelID uuid.UUID,
	name, language, category string,
	components json.RawMessage,
) (*Template, error) {
	creator, ok := s.meta.(interface {
		CreateMessageTemplate(context.Context, string, string, metapkg.CreateTemplateRequest) (*metapkg.CreateTemplateResponse, error)
	})
	if !ok || s.meta == nil {
		return nil, serviceerrors.NewValidation("create", "Meta template create client is not configured")
	}

	ch, creds, token, err := s.loadChannelCredentials(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	if ch.ChannelType == channel.TypeInstagram {
		return nil, serviceerrors.NewValidation("channel_id", "message templates are only supported for WhatsApp channels")
	}
	if err := channel.ValidateForMeta(ch, creds, token); err != nil {
		return nil, err
	}

	components, err = s.resolveStorageHeaderHandles(ctx, teamID, token, components)
	if err != nil {
		return nil, err
	}
	createReq, err := ToMetaCreateRequest(name, language, category, components)
	if err != nil {
		return nil, serviceerrors.NewValidation("components", "failed to prepare components for Meta")
	}

	wabaID := stringValue(ch.WABAID)
	var created *metapkg.CreateTemplateResponse
	err = metapkg.Retry(ctx, func(retryCtx context.Context) error {
		var retryErr error
		created, retryErr = creator.CreateMessageTemplate(retryCtx, wabaID, token, createReq)
		return retryErr
	})
	if err != nil {
		if metapkg.IsInvalidToken(err) {
			return nil, serviceerrors.NewValidation("channel_id", "Meta access token is invalid or expired")
		}
		return nil, serviceerrors.NewValidation("meta", formatMetaTemplateCreateError(err))
	}
	if created == nil || strings.TrimSpace(created.ID) == "" {
		return nil, serviceerrors.NewValidation("meta", "Meta did not return a template id")
	}

	tmpl := MapFromMetaCreate(
		channelID, teamID, wabaID, name, language, category,
		components, *created, time.Now().UTC(),
	)
	if err := s.templates.UpsertFromSync(ctx, tmpl); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	existing, findErr := s.templates.FindByMetaTemplateID(ctx, teamID, channelID, created.ID)
	if findErr == nil {
		return existing, nil
	}
	return tmpl, nil
}

func (s *SyncService) resolveStorageHeaderHandles(
	ctx context.Context,
	teamID uuid.UUID,
	accessToken string,
	components json.RawMessage,
) (json.RawMessage, error) {
	var items []map[string]any
	if err := json.Unmarshal(components, &items); err != nil {
		return nil, serviceerrors.NewValidation("components", "components must be a JSON array")
	}

	changed := false
	for _, item := range items {
		typeRaw, _ := item["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typeRaw), "HEADER") {
			continue
		}
		format, _ := item["format"].(string)
		format = strings.ToUpper(strings.TrimSpace(format))
		if format == "" || format == "TEXT" {
			continue
		}
		example, _ := item["example"].(map[string]any)
		if example == nil {
			continue
		}
		handles, _ := example["header_handle"].([]any)
		if len(handles) == 0 {
			continue
		}
		resolved := make([]any, 0, len(handles))
		storageObjectIDs := make([]any, 0, len(handles))
		for _, handle := range handles {
			value, _ := handle.(string)
			value = strings.TrimSpace(value)
			if value == "" {
				return nil, serviceerrors.NewValidation("components", "media HEADER header_handle is empty")
			}
			if !uuidLikeHandle.MatchString(value) {
				resolved = append(resolved, value)
				continue
			}
			storageObjectIDs = append(storageObjectIDs, value)
			metaHandle, err := s.uploadStorageObjectAsHeaderHandle(ctx, teamID, accessToken, format, value)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, metaHandle)
			changed = true
		}
		example["header_handle"] = resolved
		if len(storageObjectIDs) > 0 {
			example["storage_object_id"] = storageObjectIDs
		}
		item["example"] = example
	}
	if !changed {
		return components, nil
	}
	out, err := json.Marshal(items)
	if err != nil {
		return nil, serviceerrors.NewValidation("components", "failed to rewrite media header handles")
	}
	return out, nil
}

func (s *SyncService) uploadStorageObjectAsHeaderHandle(
	ctx context.Context,
	teamID uuid.UUID,
	accessToken, headerFormat, objectIDRaw string,
) (string, error) {
	if strings.TrimSpace(s.appID) == "" {
		return "", serviceerrors.NewValidation("meta", "META_APP_ID is required to upload template header media")
	}
	if s.storage == nil {
		return "", serviceerrors.NewValidation("components", "storage-service is not configured for template header media")
	}
	uploader, ok := s.meta.(resumableMediaUploader)
	if !ok || uploader == nil {
		return "", serviceerrors.NewValidation("create", "Meta resumable upload client is not configured")
	}

	objectID, err := uuid.Parse(objectIDRaw)
	if err != nil {
		return "", serviceerrors.NewValidation("components", "invalid storage object id in header_handle")
	}

	var handle string
	err = metapkg.Retry(ctx, func(retryCtx context.Context) error {
		reader, meta, downloadErr := s.storage.Download(retryCtx, teamID, objectID)
		if downloadErr != nil {
			return downloadErr
		}
		defer reader.Close()

		mimeType := strings.TrimSpace(meta.MimeType)
		if mimeType == "" {
			mimeType = defaultMimeForHeaderFormat(headerFormat)
		}
		if mimeErr := validateHeaderMime(headerFormat, mimeType); mimeErr != nil {
			return mimeErr
		}
		fileName := path.Base(strings.TrimSpace(meta.ObjectKey))
		if fileName == "" || fileName == "." || fileName == "/" {
			fileName = "template-header"
		}
		size := meta.SizeBytes
		if size <= 0 {
			return serviceerrors.NewValidation("components", "header media size is unknown")
		}

		uploadedHandle, uploadErr := uploader.UploadResumableMedia(
			retryCtx,
			s.appID,
			accessToken,
			fileName,
			mimeType,
			reader,
			size,
		)
		if uploadErr != nil {
			return uploadErr
		}
		handle = uploadedHandle
		return nil
	})
	if err != nil {
		if metapkg.IsInvalidToken(err) {
			return "", serviceerrors.NewValidation("channel_id", "Meta access token is invalid or expired")
		}
		if serviceerrors.IsValidation(err) {
			return "", err
		}
		return "", serviceerrors.NewValidation("meta", fmt.Sprintf("failed to upload header media to Meta: %v", err))
	}
	if strings.TrimSpace(handle) == "" {
		return "", serviceerrors.NewValidation("meta", "Meta did not return a header media handle")
	}
	return handle, nil
}

func defaultMimeForHeaderFormat(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "IMAGE":
		return "image/jpeg"
	case "VIDEO":
		return "video/mp4"
	case "DOCUMENT":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func validateHeaderMime(format, mimeType string) error {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "IMAGE":
		if mimeType == "image/jpeg" || mimeType == "image/jpg" || mimeType == "image/png" {
			return nil
		}
		return serviceerrors.NewValidation("components", "IMAGE header must be JPEG or PNG")
	case "VIDEO":
		if mimeType == "video/mp4" {
			return nil
		}
		return serviceerrors.NewValidation("components", "VIDEO header must be MP4")
	case "DOCUMENT":
		if mimeType == "application/pdf" {
			return nil
		}
		return serviceerrors.NewValidation("components", "DOCUMENT header must be PDF")
	default:
		return serviceerrors.NewValidation("components", "unsupported HEADER format")
	}
}

func isValidTemplateName(name string) bool {
	if len(name) == 0 || len(name) > 512 {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func normalizeCreateLanguage(raw string) string {
	return DefaultLanguageResolver().ForStorageFromCreate(raw)
}

func normalizeCreateCategory(raw string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "MARKETING":
		return "MARKETING", nil
	case "UTILITY":
		return "UTILITY", nil
	case "AUTHENTICATION":
		return "AUTHENTICATION", nil
	default:
		return "", serviceerrors.NewValidation("category", "category must be MARKETING, UTILITY, or AUTHENTICATION")
	}
}

func normalizeCreateComponents(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, serviceerrors.NewValidation("components", "components are required")
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, serviceerrors.NewValidation("components", "components must be a JSON array")
	}
	if len(items) == 0 {
		return nil, serviceerrors.NewValidation("components", "at least one component is required")
	}

	hasBody := false
	for _, item := range items {
		typeRaw, _ := item["type"].(string)
		switch strings.ToUpper(strings.TrimSpace(typeRaw)) {
		case "BODY":
			hasBody = true
		case "HEADER":
			if err := validateHeaderComponent(item); err != nil {
				return nil, err
			}
		}
	}
	if !hasBody {
		return nil, serviceerrors.NewValidation("components", "BODY component is required")
	}

	out, err := json.Marshal(items)
	if err != nil {
		return nil, serviceerrors.NewValidation("components", "failed to normalize components")
	}
	return out, nil
}

func validateHeaderComponent(item map[string]any) error {
	format, _ := item["format"].(string)
	format = strings.ToUpper(strings.TrimSpace(format))
	if format == "" || format == "TEXT" {
		return nil
	}
	if format != "IMAGE" && format != "VIDEO" && format != "DOCUMENT" {
		return serviceerrors.NewValidation("components", "unsupported HEADER format")
	}
	example, _ := item["example"].(map[string]any)
	if example == nil {
		return serviceerrors.NewValidation(
			"components",
			"media HEADER requires header_handle (storage object id or Meta upload handle)",
		)
	}
	handles, _ := example["header_handle"].([]any)
	if len(handles) == 0 {
		return serviceerrors.NewValidation(
			"components",
			"media HEADER requires header_handle (storage object id or Meta upload handle)",
		)
	}
	for _, handle := range handles {
		value, _ := handle.(string)
		if strings.TrimSpace(value) == "" {
			return serviceerrors.NewValidation("components", "media HEADER header_handle is empty")
		}
	}
	return nil
}

func formatMetaTemplateCreateError(err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "failed to create template in Meta"
	}
	return fmt.Sprintf("failed to create template in Meta: %s", msg)
}
