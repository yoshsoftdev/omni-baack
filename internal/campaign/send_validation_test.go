package campaign

import (
	"encoding/json"
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestBuildTemplateComponentsBodyOrder(t *testing.T) {
	componentsJSON := json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}, email {{2}}"}]`)
	variables := map[string]string{
		"body.1": "John",
		"body.2": "john@example.com",
	}

	components, err := BuildTemplateComponentsFromSnapshot(componentsJSON, VariableMapping{
		Body: map[string]string{
			"2": "email",
			"1": "name",
		},
	}, variables)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}
	if components[0].Type != "body" {
		t.Fatalf("expected body component")
	}
	if len(components[0].Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(components[0].Parameters))
	}
	if components[0].Parameters[0].Text != "John" {
		t.Fatalf("expected first param John, got %s", components[0].Parameters[0].Text)
	}
	if components[0].Parameters[1].Text != "john@example.com" {
		t.Fatalf("expected second param email, got %s", components[0].Parameters[1].Text)
	}
}

func TestValidateMissingVariableNoMetaCall(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}"}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{}, map[string]string{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	valErr, ok := err.(*SendValidationError)
	if !ok {
		t.Fatalf("expected SendValidationError, got %T", err)
	}
	if valErr.Code != ValidationCodeMissingVariable {
		t.Fatalf("expected missing_variable, got %s", valErr.Code)
	}
}

func TestValidateMissingSnapshotVariableNoContactFallback(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}"}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{
		Body: map[string]string{"1": "name"},
	}, map[string]string{})
	if err == nil {
		t.Fatal("expected missing snapshot variable error")
	}
	if !IsSendValidationError(err) {
		t.Fatalf("expected SendValidationError, got %v", err)
	}
}

func TestValidateInvalidMediaURL(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{
		Header: &HeaderMapping{MediaLink: "not-a-url"},
	}, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	valErr := err.(*SendValidationError)
	if valErr.Code != ValidationCodeInvalidMediaURL {
		t.Fatalf("expected invalid_media_url, got %s", valErr.Code)
	}
}

func TestValidateDualMediaReference(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{
		Header: &HeaderMapping{MediaLink: "https://cdn.example.com/a.jpg", MediaID: "123"},
	}, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	valErr := err.(*SendValidationError)
	if valErr.Code != ValidationCodeInvalidMediaReference {
		t.Fatalf("expected invalid_media_reference, got %s", valErr.Code)
	}
}

func TestValidateMediaIDAccepted(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`),
	}
	if err := ValidateTemplateSend(tpl, VariableMapping{
		Header: &HeaderMapping{MediaID: "1234567890"},
	}, nil); err != nil {
		t.Fatalf("expected numeric media id to pass validation: %v", err)
	}
}

func TestValidateHeaderHandleRejectedAsMediaID(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{
		Header: &HeaderMapping{MediaID: "4::abc"},
	}, nil)
	if err == nil {
		t.Fatal("expected validation error for template header_handle used as media_id")
	}
	valErr := err.(*SendValidationError)
	if valErr.Code != ValidationCodeInvalidMediaReference {
		t.Fatalf("expected invalid_media_reference, got %s", valErr.Code)
	}
}

func TestValidateURLButtonRequiresMapping(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"BUTTONS","buttons":[{"type":"URL","text":"Track","url":"https://example.com/{{1}}"}]}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{}, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	valErr := err.(*SendValidationError)
	if valErr.Code != ValidationCodeInvalidButtonParameter {
		t.Fatalf("expected invalid_button_parameter, got %s", valErr.Code)
	}
}

func TestValidateCopyCodeRequired(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"BUTTONS","buttons":[{"type":"COPY_CODE","text":"Copy"}]}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{}, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	valErr := err.(*SendValidationError)
	if valErr.Code != ValidationCodeInvalidButtonParameter {
		t.Fatalf("expected invalid_button_parameter, got %s", valErr.Code)
	}
}
