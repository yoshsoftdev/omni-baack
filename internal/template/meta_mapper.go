package template

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var variablePattern = regexp.MustCompile(`\{\{\d+\}\}`)

// ---------------------------------------------------------------------------
// Meta ↔ Domain boundary
//
// All conversion between Graph API template DTOs and the Template domain MUST
// go through these functions. Campaign / Inbox / Worker must not construct or
// parse Meta template DTOs directly.
// ---------------------------------------------------------------------------

// MapFromMeta converts a Meta list/sync MessageTemplate into the domain Template.
// Internal metadata starts empty (Meta never returns storage_object_id).
func MapFromMeta(
	channelID, teamID uuid.UUID,
	wabaID string,
	meta metapkg.MessageTemplate,
	syncedAt time.Time,
) *Template {
	status := mapMetaStatus(meta.Status)
	language := DefaultLanguageResolver().ForStorageFromMeta(meta.Language)
	components := meta.Components
	if len(components) == 0 {
		components = json.RawMessage("[]")
	}
	preview := extractBodyPreview(components)
	categoryPtr := mapMetaCategoryPtr(meta.Category)

	return &Template{
		ID:                 uuid.New(),
		ChannelID:          channelID,
		TeamID:             teamID,
		WABAID:             wabaID,
		MetaTemplateID:     strings.TrimSpace(meta.ID),
		Name:               strings.TrimSpace(meta.Name),
		Language:           language,
		NormalizedLanguage: DefaultLanguageResolver().NormalizedStorageKey(language),
		Category:           categoryPtr,
		Status:             status,
		Preview:            preview,
		Components:         components,
		VariableCount:      countVariables(components),
		Metadata:           EmptyMetadata(),
		SyncVersion:        0,
		LastSyncAt:         &syncedAt,
		CreatedAt:          syncedAt,
		UpdatedAt:          syncedAt,
	}
}

// ToMetaCreateRequest builds a Graph CreateTemplateRequest from domain create fields.
// domainComponents may include internal keys (e.g. storage_object_id); those are stripped.
func ToMetaCreateRequest(
	name, language, category string,
	domainComponents json.RawMessage,
) (metapkg.CreateTemplateRequest, error) {
	metaComponents, err := ComponentsForMetaAPI(domainComponents)
	if err != nil {
		return metapkg.CreateTemplateRequest{}, fmt.Errorf("components for meta: %w", err)
	}
	return metapkg.CreateTemplateRequest{
		Name:       strings.TrimSpace(name),
		Language:   strings.TrimSpace(language),
		Category:   strings.TrimSpace(category),
		Components: metaComponents,
	}, nil
}

// MapFromMetaCreate builds a domain Template after a successful Meta create call.
// domainComponents are the local components (may retain storage_object_id);
// created is the Meta create response (id/status/category only).
func MapFromMetaCreate(
	channelID, teamID uuid.UUID,
	wabaID, name, language, fallbackCategory string,
	domainComponents json.RawMessage,
	created metapkg.CreateTemplateResponse,
	now time.Time,
) *Template {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	status := mapMetaStatus(created.Status)
	cat := strings.ToUpper(strings.TrimSpace(fallbackCategory))
	if strings.TrimSpace(created.Category) != "" {
		cat = strings.ToUpper(strings.TrimSpace(created.Category))
	}
	if len(domainComponents) == 0 {
		domainComponents = json.RawMessage("[]")
	}
	preview := extractBodyPreview(domainComponents)
	var categoryPtr *string
	if cat != "" {
		categoryPtr = &cat
	}
	resolver := DefaultLanguageResolver()
	language = resolver.ForStorageFromCreate(language)

	return &Template{
		ID:                 uuid.New(),
		ChannelID:          channelID,
		TeamID:             teamID,
		WABAID:             wabaID,
		MetaTemplateID:     strings.TrimSpace(created.ID),
		Name:               strings.TrimSpace(name),
		Language:           language,
		NormalizedLanguage: resolver.NormalizedStorageKey(language),
		Category:           categoryPtr,
		Status:             status,
		Preview:            preview,
		Components:         domainComponents,
		VariableCount:      countVariables(domainComponents),
		Metadata:           ExtractInternalMetadataFromComponents(domainComponents),
		SyncVersion:        0,
		LastSyncAt:         &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// ToMetaSendLanguage returns the language.code value for Meta send APIs.
func ToMetaSendLanguage(stored string) string {
	return DefaultLanguageResolver().ForMetaSend(stored)
}

// ComponentsForMetaAPI returns component JSON safe to send to Meta Graph
// (strips app-only example keys such as storage_object_id).
func ComponentsForMetaAPI(components json.RawMessage) (json.RawMessage, error) {
	return stripMetaInternalExampleFields(components)
}

// stripMetaInternalExampleFields removes app-only keys from component examples before Meta API calls.
func stripMetaInternalExampleFields(components json.RawMessage) (json.RawMessage, error) {
	if len(components) == 0 {
		return json.RawMessage("[]"), nil
	}
	var items []map[string]any
	if err := json.Unmarshal(components, &items); err != nil {
		return nil, err
	}
	changed := false
	for _, item := range items {
		example, _ := item["example"].(map[string]any)
		if example == nil {
			continue
		}
		if _, ok := example["storage_object_id"]; ok {
			delete(example, "storage_object_id")
			item["example"] = example
			changed = true
		}
	}
	if !changed {
		return components, nil
	}
	return json.Marshal(items)
}

func mapMetaStatus(raw string) Status {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case string(StatusApproved):
		return StatusApproved
	case string(StatusRejected):
		return StatusRejected
	default:
		return StatusPending
	}
}

// normalizeMetaStatus is kept for callers inside the template package (create legacy paths).
func normalizeMetaStatus(raw string) Status { return mapMetaStatus(raw) }

// mapMetaLanguageInbound maps Meta list/sync language into the stored domain form.
func mapMetaLanguageInbound(raw string) string {
	return DefaultLanguageResolver().ForStorageFromMeta(raw)
}

// normalizeMetaLanguage is retained for legacy call sites; prefer LanguageResolver.
func normalizeMetaLanguage(raw string) string {
	return DefaultLanguageResolver().ForStorageFromMeta(raw)
}

func mapMetaCategoryPtr(raw string) *string {
	category := strings.TrimSpace(raw)
	if category == "" {
		return nil
	}
	return &category
}

func extractBodyPreview(components json.RawMessage) *string {
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(components, &items); err != nil {
		return nil
	}
	for _, item := range items {
		if strings.EqualFold(item.Type, "BODY") && strings.TrimSpace(item.Text) != "" {
			text := strings.TrimSpace(item.Text)
			return &text
		}
	}
	return nil
}

func countVariables(components json.RawMessage) int {
	return len(variablePattern.FindAllString(string(components), -1))
}
