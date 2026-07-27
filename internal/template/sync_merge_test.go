package template

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestMergeComponentsPreservingInternal(t *testing.T) {
	t.Parallel()
	existing := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::old"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hi {{1}}"}
	]`)
	incoming := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::new"]}},
		{"type":"BODY","text":"Hello {{1}}"}
	]`)
	merged := MergeComponentsPreservingInternal(existing, incoming)
	var items []map[string]any
	if err := json.Unmarshal(merged, &items); err != nil {
		t.Fatal(err)
	}
	example, _ := items[0]["example"].(map[string]any)
	ids := coerceStringList(example["storage_object_id"])
	if len(ids) != 1 || ids[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("storage_object_id = %#v", example["storage_object_id"])
	}
	handles, _ := example["header_handle"].([]any)
	if len(handles) != 1 || handles[0] != "4::new" {
		t.Fatalf("header_handle should come from Meta: %#v", handles)
	}
	body, _ := items[1]["text"].(string)
	if body != "Hello {{1}}" {
		t.Fatalf("body = %s", body)
	}
}

func TestMergeSyncedTemplatePreservesIDAndMetadata(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	storageID := "11111111-1111-1111-1111-111111111111"
	existing := &Template{
		ID:         id,
		Name:       "welcome",
		Language:   "en_US",
		Status:     StatusApproved,
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"storage_object_id":["`+storageID+`"]}},{"type":"BODY","text":"Old"}]`),
		Metadata:   EmptyMetadata().WithStorageObjectIDs([]string{storageID}),
	}
	incoming := &Template{
		ID:         uuid.New(),
		Name:       "welcome_v2",
		Language:   "en_US",
		Status:     StatusApproved,
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::x"]}},{"type":"BODY","text":"New"}]`),
		Metadata:   EmptyMetadata(),
	}
	merged := MergeSyncedTemplate(existing, incoming)
	if merged.ID != id {
		t.Fatalf("id = %s", merged.ID)
	}
	if merged.Name != "welcome_v2" {
		t.Fatalf("name = %s", merged.Name)
	}
	ids := merged.Metadata.StorageObjectIDs()
	if len(ids) != 1 || ids[0] != storageID {
		t.Fatalf("metadata ids = %#v", ids)
	}
	fromComponents := ExtractInternalMetadataFromComponents(merged.Components).StorageObjectIDs()
	if len(fromComponents) != 1 || fromComponents[0] != storageID {
		t.Fatalf("component storage ids = %#v", fromComponents)
	}
}

func TestInjectStorageObjectIDsIntoComponentsFromMetadataOnly(t *testing.T) {
	t.Parallel()
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::x"]}},{"type":"BODY","text":"Hi"}]`)
	out := InjectStorageObjectIDsIntoComponents(components, []string{"11111111-1111-1111-1111-111111111111"})
	ids := ExtractInternalMetadataFromComponents(out).StorageObjectIDs()
	if len(ids) != 1 {
		t.Fatalf("ids = %#v", ids)
	}
}
