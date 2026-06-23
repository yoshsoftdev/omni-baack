//go:build legacy

package campaign

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestRenderTemplateContent(t *testing.T) {
	components, _ := json.Marshal([]map[string]string{
		{"type": "BODY", "text": "Halo {{1}}, kode Anda {{2}}."},
	})
	tpl := &template.Template{
		ID:         uuid.New(),
		Name:       "kode_otp",
		Components: components,
	}
	c := &contact.Contact{Name: "YoShandy", Phone: "+6282288812003"}

	content := renderTemplateContent(tpl, VariableMapping{
		Body: map[string]string{
			"1": "name",
			"2": "phone",
		},
	}, c)

	want := "Halo YoShandy, kode Anda +6282288812003."
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestRenderTemplateContent_FallbackName(t *testing.T) {
	tpl := &template.Template{Name: "promo_sale", Components: json.RawMessage(`[]`)}
	content := renderTemplateContent(tpl, VariableMapping{}, &contact.Contact{})
	if content != "[Template: promo_sale]" {
		t.Fatalf("content = %q", content)
	}
}
