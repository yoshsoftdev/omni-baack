package template

import (
	"encoding/json"
)

// Metadata holds internal Template fields that must not be overwritten by Meta sync
// and must never be sent to the Graph API (e.g. storage_object_id, custom flags).
//
// Stored as JSONB. Empty metadata is "{}".
type Metadata json.RawMessage

// EmptyMetadata returns a non-nil empty metadata object.
func EmptyMetadata() Metadata {
	return Metadata(`{}`)
}

// Normalize returns EmptyMetadata when m is nil or empty.
func (m Metadata) Normalize() Metadata {
	if len(m) == 0 || string(m) == "null" {
		return EmptyMetadata()
	}
	return m
}

// Raw returns the underlying JSON bytes (never nil after Normalize).
func (m Metadata) Raw() json.RawMessage {
	return json.RawMessage(m.Normalize())
}

// Map decodes metadata into a generic map. Invalid JSON yields an empty map.
func (m Metadata) Map() map[string]any {
	out := map[string]any{}
	raw := m.Normalize()
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

// MetadataFromMap encodes a map as Metadata. Nil/empty map → EmptyMetadata.
func MetadataFromMap(v map[string]any) Metadata {
	if len(v) == 0 {
		return EmptyMetadata()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return EmptyMetadata()
	}
	return Metadata(b)
}

// MergeMetadata overlays patch keys onto base. Patch wins on key conflict.
// Used by Phase 7 sync merge; available on the domain model now.
func MergeMetadata(base, patch Metadata) Metadata {
	merged := base.Map()
	for k, v := range patch.Map() {
		merged[k] = v
	}
	return MetadataFromMap(merged)
}

// StorageObjectIDs returns header media storage object id strings if present.
// Keys: "storage_object_id" as string or []any of strings (domain convention).
func (m Metadata) StorageObjectIDs() []string {
	v, ok := m.Map()["storage_object_id"]
	if !ok || v == nil {
		return nil
	}
	switch typed := v.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// WithStorageObjectIDs returns a copy of metadata with storage_object_id set.
func (m Metadata) WithStorageObjectIDs(ids []string) Metadata {
	mp := m.Map()
	if len(ids) == 0 {
		delete(mp, "storage_object_id")
	} else {
		anyIDs := make([]any, len(ids))
		for i, id := range ids {
			anyIDs[i] = id
		}
		mp["storage_object_id"] = anyIDs
	}
	return MetadataFromMap(mp)
}

// ExtractInternalMetadataFromComponents copies internal-only keys from component JSON
// into Metadata so they survive Meta sync overwrites of components (Phase 7 completes merge).
func ExtractInternalMetadataFromComponents(components json.RawMessage) Metadata {
	if len(components) == 0 {
		return EmptyMetadata()
	}
	var items []struct {
		Type    string         `json:"type"`
		Example map[string]any `json:"example"`
	}
	if err := json.Unmarshal(components, &items); err != nil {
		return EmptyMetadata()
	}
	meta := EmptyMetadata()
	for _, item := range items {
		if item.Example == nil {
			continue
		}
		if raw, ok := item.Example["storage_object_id"]; ok && raw != nil {
			ids := coerceStringList(raw)
			if len(ids) > 0 {
				meta = meta.WithStorageObjectIDs(ids)
			}
		}
	}
	return meta
}

func coerceStringList(v any) []string {
	switch typed := v.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
