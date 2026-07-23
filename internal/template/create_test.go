package template

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCreateComponentsRequiresBody(t *testing.T) {
	_, err := normalizeCreateComponents(json.RawMessage(`[{"type":"FOOTER","text":"x"}]`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeCreateComponentsAcceptsStorageUUIDHandle(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Halo"}
	]`)
	out, err := normalizeCreateComponents(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected components")
	}
}

func TestNormalizeCreateComponentsAcceptsTextHeader(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"HEADER","format":"TEXT","text":"Promo"},
		{"type":"BODY","text":"Halo {{1}}"},
		{"type":"FOOTER","text":"Skinsure"},
		{"type":"BUTTONS","buttons":[{"type":"URL","text":"Shop","url":"https://example.com"}]}
	]`)
	out, err := normalizeCreateComponents(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected components")
	}
}

func TestNormalizeCreateCategory(t *testing.T) {
	got, err := normalizeCreateCategory("marketing")
	if err != nil || got != "MARKETING" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestIsValidTemplateName(t *testing.T) {
	if !isValidTemplateName("promo_juni_2024") {
		t.Fatal("expected valid")
	}
	if isValidTemplateName("Promo Juni") {
		t.Fatal("expected invalid")
	}
}

func TestStripMetaInternalExampleFieldsRemovesStorageObjectID(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hi"}
	]`)
	out, err := stripMetaInternalExampleFields(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	example, _ := items[0]["example"].(map[string]any)
	if example == nil {
		t.Fatal("expected header example")
	}
	if _, ok := example["storage_object_id"]; ok {
		t.Fatal("storage_object_id must not be sent to Meta")
	}
	handles, _ := example["header_handle"].([]any)
	if len(handles) != 1 || handles[0] != "4::abc" {
		t.Fatalf("header_handle = %#v", handles)
	}
}
