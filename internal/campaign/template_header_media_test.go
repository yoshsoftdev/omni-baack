package campaign

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
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

func TestApplyTemplateHeaderMediaDefaultsDoesNotUseHandleAsMediaID(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::meta-handle-1"]}}]`)
	tpl := &template.Template{Components: components}

	mapping := ApplyTemplateHeaderMediaDefaults(VariableMapping{}, tpl)
	if headerMappingHasSendableMedia(mapping.Header) {
		t.Fatalf("header_handle must not become send media_id, got %+v", mapping.Header)
	}
}

func TestApplyTemplateHeaderMediaDefaultsKeepsCampaignOverride(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::meta-handle-1"]}}]`)
	tpl := &template.Template{Components: components}

	mapping := ApplyTemplateHeaderMediaDefaults(VariableMapping{
		Header: &HeaderMapping{MediaLink: "https://cdn.example.com/banner.jpg"},
	}, tpl)
	if mapping.Header.MediaLink != "https://cdn.example.com/banner.jpg" {
		t.Fatalf("expected campaign media link to win, got %+v", mapping.Header)
	}
}

func TestValidateTemplateSendRequiresHeaderMediaWithoutStorage(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::meta-handle-1"]}},{"type":"BODY","text":"Hi"}]`)
	tpl := &template.Template{Components: components}

	err := ValidateTemplateSend(tpl, VariableMapping{}, nil, nil)
	if err == nil {
		t.Fatal("expected validation error when only template header_handle is present")
	}
	valErr, ok := err.(*SendValidationError)
	if !ok {
		t.Fatalf("expected SendValidationError, got %T", err)
	}
	if valErr.Code != ValidationCodeMissingVariable {
		t.Fatalf("expected missing_variable, got %s", valErr.Code)
	}
}

func TestValidateTemplateSendAllowsStorageBackedHeaderDefault(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},{"type":"BODY","text":"Hi"}]`)
	tpl := &template.Template{Components: components}

	if err := ValidateTemplateSend(tpl, VariableMapping{}, nil, nil); err != nil {
		t.Fatalf("expected storage-backed header to pass pre-send validation: %v", err)
	}
}

func TestIsWhatsAppCloudMediaID(t *testing.T) {
	if !IsWhatsAppCloudMediaID("1234567890") {
		t.Fatal("expected numeric media id")
	}
	if IsWhatsAppCloudMediaID("4::abc") {
		t.Fatal("header_handle must not be treated as cloud media id")
	}
	if IsWhatsAppCloudMediaID("meta-handle-1") {
		t.Fatal("non-numeric id must be rejected")
	}
}

func TestResolveHeaderMediaForSendClearsInvalidMediaID(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"]}}]`)
	tpl := &template.Template{Components: components}

	mapping, err := ResolveHeaderMediaForSend(
		t.Context(),
		VariableMapping{Header: &HeaderMapping{MediaID: "4::abc"}},
		tpl,
		uuidMust("11111111-1111-1111-1111-111111111111"),
		"phone",
		"token",
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Header == nil || mapping.Header.MediaID != "" {
		t.Fatalf("expected invalid media_id cleared, got %+v", mapping.Header)
	}
}

func uuidMust(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}
