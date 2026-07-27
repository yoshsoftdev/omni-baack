package builder_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
)

func TestBuildBodyOnly(t *testing.T) {
	t.Parallel()
	tmpl := &template.Template{
		ID:         uuid.New(),
		Components: json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}"}]`),
	}
	components, err := builder.Build(tmpl, builder.Variables{
		Body: map[string]string{"1": "Alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || components[0].Type != "body" {
		t.Fatalf("components = %#v", components)
	}
	if components[0].Parameters[0].Text != "Alice" {
		t.Fatalf("text = %s", components[0].Parameters[0].Text)
	}
}

func TestRequiredFromHeaderMedia(t *testing.T) {
	t.Parallel()
	tmpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE"},{"type":"BODY","text":"Hi {{1}}"}]`),
	}
	req, err := builder.RequiredFrom(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if !req.HeaderMedia {
		t.Fatal("expected header media")
	}
	if _, ok := req.Body["1"]; !ok {
		t.Fatal("expected body {{1}}")
	}
}

func TestMediaRefValidate(t *testing.T) {
	t.Parallel()
	if err := (builder.MediaRef{}).Validate(); err == nil {
		t.Fatal("expected error")
	}
	if err := (builder.MediaRef{Link: "https://x", MediaID: "1"}).Validate(); err == nil {
		t.Fatal("expected dual ref error")
	}
	if err := (builder.MediaRef{MediaID: "123"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
