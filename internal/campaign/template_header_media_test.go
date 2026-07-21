package campaign

import (
	"encoding/json"
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestExtractTemplateHeaderMedia(t *testing.T) {
	components := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hi {{1}}"}
	]`)

	media, ok := ExtractTemplateHeaderMedia(components)
	if !ok {
		t.Fatal("expected header media")
	}
	if media.Handle != "4::abc" {
		t.Fatalf("handle = %q", media.Handle)
	}
	if media.StorageObjectID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("storage object id = %q", media.StorageObjectID)
	}
}

func TestApplyTemplateHeaderMediaDefaultsUsesTemplateHandle(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["meta-handle-1"]}}]`)
	tpl := &template.Template{Components: components}

	mapping := ApplyTemplateHeaderMediaDefaults(VariableMapping{}, tpl)
	if mapping.Header == nil || mapping.Header.MediaID != "meta-handle-1" {
		t.Fatalf("expected media_id from template, got %+v", mapping.Header)
	}
}

func TestApplyTemplateHeaderMediaDefaultsKeepsCampaignOverride(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["meta-handle-1"]}}]`)
	tpl := &template.Template{Components: components}

	mapping := ApplyTemplateHeaderMediaDefaults(VariableMapping{
		Header: &HeaderMapping{MediaLink: "https://cdn.example.com/banner.jpg"},
	}, tpl)
	if mapping.Header.MediaLink != "https://cdn.example.com/banner.jpg" {
		t.Fatalf("expected campaign media link to win, got %+v", mapping.Header)
	}
}

func TestValidateTemplateSendWithTemplateHeaderMediaDefault(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["meta-handle-1"]}},{"type":"BODY","text":"Hi"}]`)
	tpl := &template.Template{Components: components}

	if err := ValidateTemplateSend(tpl, VariableMapping{}, nil, nil); err != nil {
		t.Fatalf("expected template header media default to pass validation: %v", err)
	}
}
