package campaign

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestBuildTemplateComponentsBodyOrder(t *testing.T) {
	email := "john@example.com"
	c := &contact.Contact{
		ID:           uuid.New(),
		ContactName:  "John",
		CustomerName: "John",
		Phone:        "628123",
		Email:        &email,
	}
	componentsJSON := json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}, email {{2}}"}]`)

	components, err := BuildTemplateComponents(componentsJSON, VariableMapping{
		Body: map[string]string{
			"2": "email",
			"1": "name",
		},
	}, c, nil)
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
	err := ValidateTemplateSend(tpl, VariableMapping{}, &contact.Contact{CustomerName: "A", ContactName: "A"}, nil)
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

func TestValidateInvalidMediaURL(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{
		Header: &HeaderMapping{MediaLink: "not-a-url"},
	}, nil, nil)
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
	}, nil, nil)
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
		Header: &HeaderMapping{MediaID: "meta-handle-1"},
	}, nil, nil); err != nil {
		t.Fatalf("expected media id to pass validation: %v", err)
	}
}

func TestValidateURLButtonRequiresMapping(t *testing.T) {
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"BUTTONS","buttons":[{"type":"URL","text":"Track","url":"https://example.com/{{1}}"}]}]`),
	}
	err := ValidateTemplateSend(tpl, VariableMapping{}, nil, nil)
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
	err := ValidateTemplateSend(tpl, VariableMapping{}, nil, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	valErr := err.(*SendValidationError)
	if valErr.Code != ValidationCodeInvalidButtonParameter {
		t.Fatalf("expected invalid_button_parameter, got %s", valErr.Code)
	}
}
