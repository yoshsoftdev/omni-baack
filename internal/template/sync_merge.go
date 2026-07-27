package template

import (
	"encoding/json"
	"strings"
)

// MergeSyncedTemplate applies Meta sync onto an existing domain Template.
//
// Rules:
//   - Meta fields (name, language, status, category, preview, components shape) come from incoming.
//   - Internal metadata (storage_object_id, flags) is preserved from existing.
//   - Components are merged so Meta updates apply but example.storage_object_id survives.
//   - Existing primary key / created_at are preserved.
func MergeSyncedTemplate(existing, incoming *Template) *Template {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		ensureDomainDefaults(incoming)
		incoming.Metadata = MergeMetadata(
			ExtractInternalMetadataFromComponents(incoming.Components),
			incoming.Metadata,
		)
		return incoming
	}

	mergedComponents := MergeComponentsPreservingInternal(existing.Components, incoming.Components)
	// Prefer existing metadata; overlay anything still present on components after merge.
	mergedMeta := MergeMetadata(existing.Metadata, ExtractInternalMetadataFromComponents(mergedComponents))
	mergedComponents = InjectStorageObjectIDsIntoComponents(mergedComponents, mergedMeta.StorageObjectIDs())

	out := *incoming
	out.ID = existing.ID
	out.CreatedAt = existing.CreatedAt
	out.Components = mergedComponents
	out.Metadata = mergedMeta
	out.VariableCount = countVariables(mergedComponents)
	if out.Preview == nil {
		out.Preview = extractBodyPreview(mergedComponents)
	}
	out.NormalizedLanguage = DefaultLanguageResolver().NormalizedStorageKey(out.Language)
	out.SyncVersion = existing.SyncVersion // repository increments on upsert
	ensureDomainDefaults(&out)
	return &out
}

// MergeComponentsPreservingInternal overlays Meta components onto existing while keeping
// app-only example keys (storage_object_id) from the matching HEADER component.
func MergeComponentsPreservingInternal(existing, incoming json.RawMessage) json.RawMessage {
	if len(incoming) == 0 {
		return existing
	}
	var inItems []map[string]any
	if err := json.Unmarshal(incoming, &inItems); err != nil {
		return incoming
	}
	var exItems []map[string]any
	_ = json.Unmarshal(existing, &exItems)

	existingStorageByHeader := extractHeaderStorageByKey(exItems)

	changed := false
	for _, item := range inItems {
		typeRaw, _ := item["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typeRaw), "HEADER") {
			continue
		}
		format, _ := item["format"].(string)
		format = strings.ToUpper(strings.TrimSpace(format))
		if format == "" || format == "TEXT" {
			continue
		}
		key := headerMergeKey(typeRaw, format)
		example, _ := item["example"].(map[string]any)
		if example == nil {
			example = map[string]any{}
		}
		if hasNonEmptyStorageObjectID(example) {
			continue
		}
		if ids, ok := existingStorageByHeader[key]; ok && len(ids) > 0 {
			anyIDs := make([]any, len(ids))
			for i, id := range ids {
				anyIDs[i] = id
			}
			example["storage_object_id"] = anyIDs
			item["example"] = example
			changed = true
		}
	}
	if !changed {
		return incoming
	}
	out, err := json.Marshal(inItems)
	if err != nil {
		return incoming
	}
	return out
}

// InjectStorageObjectIDsIntoComponents writes metadata storage ids into the first media HEADER
// that lacks storage_object_id (recovery when components were wiped before metadata existed).
func InjectStorageObjectIDsIntoComponents(components json.RawMessage, ids []string) json.RawMessage {
	if len(components) == 0 || len(ids) == 0 {
		return components
	}
	var items []map[string]any
	if err := json.Unmarshal(components, &items); err != nil {
		return components
	}
	changed := false
	for _, item := range items {
		typeRaw, _ := item["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typeRaw), "HEADER") {
			continue
		}
		format, _ := item["format"].(string)
		format = strings.ToUpper(strings.TrimSpace(format))
		if format != "IMAGE" && format != "VIDEO" && format != "DOCUMENT" {
			continue
		}
		example, _ := item["example"].(map[string]any)
		if example == nil {
			example = map[string]any{}
		}
		if hasNonEmptyStorageObjectID(example) {
			return components
		}
		anyIDs := make([]any, len(ids))
		for i, id := range ids {
			anyIDs[i] = id
		}
		example["storage_object_id"] = anyIDs
		item["example"] = example
		changed = true
		break
	}
	if !changed {
		return components
	}
	out, err := json.Marshal(items)
	if err != nil {
		return components
	}
	return out
}

func extractHeaderStorageByKey(items []map[string]any) map[string][]string {
	out := make(map[string][]string)
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
		ids := coerceStringList(example["storage_object_id"])
		if len(ids) == 0 {
			continue
		}
		out[headerMergeKey(typeRaw, format)] = ids
	}
	return out
}

func headerMergeKey(typeRaw, format string) string {
	return strings.ToUpper(strings.TrimSpace(typeRaw)) + ":" + strings.ToUpper(strings.TrimSpace(format))
}

func hasNonEmptyStorageObjectID(example map[string]any) bool {
	return len(coerceStringList(example["storage_object_id"])) > 0
}
