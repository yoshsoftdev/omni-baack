package template

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestParseCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want Category
		ok   bool
	}{
		{"MARKETING", CategoryMarketing, true},
		{"marketing", CategoryMarketing, true},
		{"Utility", CategoryUtility, true},
		{"AUTHENTICATION", CategoryAuthentication, true},
		{"unknown", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseCategory(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("ParseCategory(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTemplateWorkspaceID(t *testing.T) {
	t.Parallel()
	teamID := uuid.New()
	tmpl := &Template{TeamID: teamID}
	if tmpl.WorkspaceID() != teamID {
		t.Fatalf("WorkspaceID = %s, want %s", tmpl.WorkspaceID(), teamID)
	}
	if (*Template)(nil).WorkspaceID() != uuid.Nil {
		t.Fatal("nil Template.WorkspaceID must be Nil")
	}
}

func TestTemplateCategoryValue(t *testing.T) {
	t.Parallel()
	cat := "UTILITY"
	tmpl := &Template{Category: &cat}
	got, ok := tmpl.CategoryValue()
	if !ok || got != CategoryUtility {
		t.Fatalf("CategoryValue = (%q, %v)", got, ok)
	}
}

func TestMetadataNormalizeAndMerge(t *testing.T) {
	t.Parallel()
	base := Metadata(`{"storage_object_id":["a"],"flag":true}`)
	patch := Metadata(`{"flag":false,"extra":1}`)
	merged := MergeMetadata(base, patch)
	mp := merged.Map()
	if mp["flag"] != false {
		t.Fatalf("flag = %#v", mp["flag"])
	}
	if mp["extra"].(float64) != 1 {
		t.Fatalf("extra = %#v", mp["extra"])
	}
	ids := merged.StorageObjectIDs()
	if len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("storage ids = %#v", ids)
	}
}

func TestMetadataStorageObjectIDs(t *testing.T) {
	t.Parallel()
	m := EmptyMetadata().WithStorageObjectIDs([]string{"11111111-1111-1111-1111-111111111111"})
	ids := m.StorageObjectIDs()
	if len(ids) != 1 {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestExtractInternalMetadataFromComponents(t *testing.T) {
	t.Parallel()
	components := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::x"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hi"}
	]`)
	meta := ExtractInternalMetadataFromComponents(components)
	ids := meta.StorageObjectIDs()
	if len(ids) != 1 || ids[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestEnsureDomainDefaults(t *testing.T) {
	t.Parallel()
	tmpl := &Template{Language: "en_US"}
	ensureDomainDefaults(tmpl)
	if string(tmpl.Metadata.Normalize()) != "{}" {
		t.Fatalf("metadata = %s", tmpl.Metadata)
	}
	if tmpl.NormalizedLanguage != "en_US" {
		t.Fatalf("normalized = %s", tmpl.NormalizedLanguage)
	}
	if string(tmpl.Components) != "[]" {
		t.Fatalf("components = %s", tmpl.Components)
	}
}
