package resolver_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
	"github.com/omnichannel/omni-chan-v2/internal/template/resolver"
)

func TestResolverBuildUsesTemplateBuilder(t *testing.T) {
	t.Parallel()
	r := resolver.New(nil)
	tmpl := &template.Template{
		ID:         uuid.New(),
		Components: json.RawMessage(`[{"type":"BODY","text":"Hi {{1}}"}]`),
		Language:   "en_US",
	}
	components, err := r.Build(tmpl, builder.Variables{Body: map[string]string{"1": "Ada"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || components[0].Parameters[0].Text != "Ada" {
		t.Fatalf("components = %#v", components)
	}
	if got := r.MetaSendLanguage("en_EN"); got != "en_US" {
		t.Fatalf("MetaSendLanguage = %s", got)
	}
}
