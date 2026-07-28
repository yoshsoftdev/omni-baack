package campaign

import (
	"encoding/json"
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestApplyTemplateButtonDefaults_OTPURLButtonFromBody(t *testing.T) {
	t.Parallel()
	tpl := &template.Template{
		Components: json.RawMessage(`[
			{"type":"BODY","text":"*{{1}}* adalah kode verifikasi Anda."},
			{"type":"BUTTONS","buttons":[{
				"type":"URL",
				"text":"Salin Kode",
				"url":"https://www.whatsapp.com/otp/code/?otp_type=COPY_CODE&code=otp{{1}}"
			}]}
		]`),
	}
	mapping := VariableMapping{Body: map[string]string{"1": "kode"}}
	variables := map[string]string{"body.1": "277353"}

	gotMapping, gotVars := ApplyTemplateButtonDefaults(tpl, mapping, variables)
	btn, ok := gotMapping.Buttons["0"]
	if !ok || btn.URL["1"] == "" {
		t.Fatalf("expected button 0 URL {{1}} mapped, got %#v", gotMapping.Buttons)
	}
	if gotVars["button.0.1"] != "277353" {
		t.Fatalf("expected button.0.1 snapshot, got %q", gotVars["button.0.1"])
	}

	if err := ValidateTemplateSend(tpl, VariableMapping{Body: map[string]string{"1": "kode"}}, map[string]string{"body.1": "277353"}); err != nil {
		t.Fatalf("body-only mapping should validate OTP URL button: %v", err)
	}
}

func TestApplyTemplateButtonDefaults_DoesNotOverrideExplicit(t *testing.T) {
	t.Parallel()
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"BUTTONS","buttons":[{"type":"URL","text":"Open","url":"https://x.example/{{1}}"}]}]`),
	}
	mapping := VariableMapping{
		Body: map[string]string{"1": "body_src"},
		Buttons: map[string]ButtonMapping{
			"0": {URL: map[string]string{"1": "btn_src"}},
		},
	}
	variables := map[string]string{
		"body.1":       "from-body",
		"button.0.1":   "from-button",
	}
	got, _ := ApplyTemplateButtonDefaults(tpl, mapping, variables)
	if got.Buttons["0"].URL["1"] != "btn_src" {
		t.Fatalf("explicit button mapping overwritten: %#v", got.Buttons["0"])
	}
}
